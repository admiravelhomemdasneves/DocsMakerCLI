# 4. Segurança e segredos

Duas promessas definem o produto: **o código não sai da máquina** e
**informação sensível não chega a nenhum resultado sem decisão humana**. Este
documento diz onde cada garantia é imposta, como é testada e o que fica de fora.

## 4.1 O código não sai da máquina

| Risco | Mecanismo | Onde | Teste |
|-------|-----------|------|-------|
| Enviar código a um serviço remoto | O único `llm.Client` que existe é o `OllamaClient`; não há implementação remota. Só `internal/llm` usa `net/http` | `internal/llm` | `demo.sh` verifica por pesquisa no código que só `internal/llm` importa `net/http` |
| `OLLAMA_HOST` a apontar para outra máquina | `ResolveBaseURL` só aceita `localhost`, IPs de loopback (`127/8`, `::1`) e trata `0.0.0.0`/`::` como a própria máquina | `llm/local.go` | `TestResolveBaseURL`; CLI: `TestDoc_ModelMissingAndRefusals` |
| Modelos "cloud" do Ollama (correm em servidores do Ollama) | `CheckLocalModel` recusa nomes com sufixo/tag `cloud` antes de qualquer pedido, em `pull` **e** em `doc` | `llm/local.go`, `app/model.go` | `TestCheckLocalModel`; `TestPull_RefusesCloudModelsWithoutContactingOllama` |
| Instalar software sem consentimento | `ModelPull` pede confirmação (`Confirm`) antes de instalar o Ollama e antes de descarregar o modelo; `nil` ou erro ⇒ recusado; EOF ⇒ "não" | `app/model.go`, `cli/model.go` | `TestModelCommands` (várias respostas, só `y` descarrega) |
| Executar scripts descarregados | O plano de instalação nunca faz `curl … \| sh`; no Linux não automatiza | `llm/bootstrap.go` (`PlanInstall`) | `TestPlanInstall` |
| `doc` a instalar/descarregar às escondidas | `ensureLocalModel` só arranca um Ollama já instalado (local, inofensivo); nunca instala nem descarrega | `app/model.go` | `TestDoc_ModelMissingAndRefusals` |

A única atividade de rede legítima é `model pull` (descarregar o modelo para a
máquina), e nesse sentido nada do código do utilizador é enviado.

**Fora do âmbito:** o próprio Ollama é um programa de terceiros em que se confia
(não se audita o que ele faz com a rede), e o docsmaker não é uma *sandbox* de
rede: garante o que *ele* envia, não o que outros processos fazem.

## 4.2 Informação sensível: dois portões

```
texto-fonte ─► [Portão 1: prompt] ─► modelo ─► [Portão 2: saída] ─► cache / consola / ficheiros / código
```

`secrets.Gate.Screen(ctx, where, text)` é chamado em `docgen.generate`:

1. antes do modelo, sobre o prompt (o que entra);
2. depois do modelo (e também nos acertos de cache), sobre a resposta (o que sai).

O segundo portão cobre o caso de o modelo repetir ou inventar um segredo, e o de
uma cache antiga conter algo que entretanto passou a ser tratado de outro modo.

### Deteção (`secrets.DefaultScanner`)

Regras por regex, **ordenadas por especificidade** (em sobreposições vence a mais
específica), e um detetor de entropia:

| Tipo | Regra |
|------|-------|
| `private-key` | Bloco `-----BEGIN … PRIVATE KEY-----` … `END` (ou até ao fim do texto, se truncado) |
| `aws-key` | `AKIA/ASIA/AIDA/AROA` + 16 alfanuméricos |
| `jwt` | `eyJ….eyJ….…` |
| `connection-string` | `esquema://utilizador:password@host` (jdbc, mongodb, postgres, mysql, redis, amqp, ftp, http(s)…) e URLs JDBC com `password=`/`pwd=` como parâmetro |
| `api-token` | GitHub (`ghp_`, `github_pat_`…), Slack (`xox…`), Stripe (`sk_/rk_live/test`), Google (`AIza…`) |
| `generic-secret` | `Bearer <token≥20>`; e **identificador que termina numa palavra sensível** (`password`, `secret`, `apiKey`, `token`, …) atribuído a um literal entre aspas |
| `high-entropy` | Literais de ≥20 caracteres sem espaços, que não sejam UUID, URL, caminho ou nome pontuado; se não-hex: tem de ter dígito e letra e entropia de Shannon ≥ 4,0; se hex: ≥32 caracteres e entropia ≥ 3,0 |

Escolhas anti-falsos-positivos: a palavra sensível é âncorada ao **fim** do
identificador (`tokenType = "Bearer"` e `passwordPolicy = "strict"` ficam de fora);
`looksLikePlaceholder` descarta `${…}`, `changeme`, constantes em maiúsculas, etc.

O intervalo marcado é só o **valor** (grupo de captura), para que a redação preserve
a chave: `password = "[REDACTED:generic-secret]"`.

### Decisões (`secrets.Approver`)

| Decisão | Efeito |
|---------|--------|
| `redact` | `Redact` substitui o intervalo por `[REDACTED:<tipo>]` |
| `skip` | A unidade (método/classe) é omitida: `OmittedSummary`, sem chamar o modelo, sem cache |
| `allow-once` | Incluído como está, só esta ocorrência |
| `allow-always` | Como `allow-once` e guardado na allowlist (por hash) |
| `allowlisted` | Registada quando um valor já na allowlist passa sem pergunta |
| `abort` | `ErrAborted`; interrompe a execução |

Implementações do porto: `TerminalApprover` (pergunta em stderr, para que o stdout
fique limpo para `--format json`; a opção por omissão é `redact`; EOF ⇒ erro,
nunca "sim") e `BlockApprover` (para `strict` e `--yes`: devolve `BlockedError`).
Para uma UI, basta outro `Approver` (ver [Extensibilidade](07-extensibilidade.md)).

O `Gate` pergunta **uma vez** por valor idêntico dentro do mesmo texto.

### Política (`app.secretsGate`)

| `--secrets` | `Approver` | Notas |
|-------------|-----------|-------|
| `prompt` (omissão) | o do driver; `BlockApprover` se `--yes` ou se não houver | |
| `strict` | `BlockApprover` | |
| `off` | sem `Gate` | O CLI imprime um aviso |
| outro | erro | |

Em todas as políticas com `Gate`, a **allowlist** é consultada antes do `Approver`.

### Allowlist (`secrets.FileAllowlist`)

- Guarda **SHA-256 do valor** (não o valor), tipo, local e data, em
  `<projeto>/.docsmaker/allowlist.json`; mais padrões regex opcionais.
- Padrões são **ancorados** (`^(?:p)$`) e recusados se aceitarem `""`, `x`,
  `hunter2` ou `AKIAIOSFODNN7EXAMPLE` (impede `.*`, `.+`, `[a-z0-9]+`).
- O ficheiro é lido de forma estrita (`DisallowUnknownFields`, versão 1): um ficheiro
  inválido é um **erro**, não é ignorado em silêncio.
- É **local por desenho**: o hash de um valor fraco (uma password curta) pode ser
  descoberto por tentativas, por isso o ficheiro vive em `.docsmaker/`, que `init`
  exclui do git, e não num ficheiro versionado partilhado.
- Aplica-se também em `strict`/`--yes`: é uma decisão humana anterior e explícita, e sem ela um
  falso positivo bloquearia a automação para sempre.

### Auditoria (`secrets.FileAuditLog`)

Linha por decisão em `.docsmaker/audit.log`:
`<UTC> decision=… kind=… where="…" value="jdb***db (50 chars)"`.
O valor é **sempre mascarado** (`Mask`); falhar a escrever o registo nunca derruba a execução.

## 4.3 Garantias do modo inline

Ver [Inserção e filtro git](05-insercao-e-filtro-git.md): recusa-se a escrever sem o
filtro configurado, nunca toca em comentários escritos por pessoas, sanitiza o texto do
modelo (não pode fechar o comentário, forjar uma marca ou iniciar um `\u`), verifica
por re-análise e escreve de forma atómica.

## 4.4 Como se verifica

| Camada | Teste |
|--------|-------|
| Detetores, entropia, falsos positivos | `internal/secrets/detectors_test.go` |
| Decisões, portões, auditoria sem segredo, allowlist | `gate_test.go`, `allowlist_test.go` |
| Os dois portões no pipeline (o prompt nunca tem o segredo; o eco do modelo é apanhado) | `internal/docgen/gate_test.go` |
| Pela linha de comandos, com um Ollama falso que regista os prompts | `internal/cli/cli_test.go` (`TestDoc_Secrets`) |
| De ponta a ponta com o modelo real e commits reais | `scripts/demo.sh` |

Foi feita **verificação por mutação** nos testes do CLI: desativar a allowlist ou a
redação faz falhar o teste certo.

## 4.5 Riscos residuais e limites

- **A deteção é heurística.** Há falsos positivos (resolvidos com a allowlist) e falsos
  negativos (um segredo sem padrão conhecido e com pouca entropia, partido em pedaços,
  ou em texto livre). Não substitui um *secret scanner* dedicado nem boas práticas.
- Só se analisa o **texto enviado ao modelo** e o que ele devolve; não se analisa o
  repositório inteiro.
- Com `allow-once`/`allow-always`/`--secrets off`, o valor chega ao modelo local e pode
  aparecer no resultado (o portão de saída volta a perguntar se for repetido, a menos
  que o valor esteja na allowlist).
- A cache (`.docsmaker/cache/`) contém resumos gerados do código; fica fora do git,
  mas é texto em claro no disco do utilizador.
- Quem controla o `Approver` controla as decisões; o `Gate` assume-o de confiança.
