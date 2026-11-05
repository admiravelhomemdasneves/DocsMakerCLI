# 5. Informação sensível

O código tem, por vezes, coisas que nunca deviam aparecer numa documentação:
passwords, chaves de API, tokens, ligações a bases de dados com credenciais.
O docsmaker verifica isso **duas vezes** em cada passo:

1. **Antes do modelo:** o texto que ia ser enviado ao modelo é verificado.
   (Mesmo sendo um modelo local, evita que o segredo acabe repetido na documentação.)
2. **Depois do modelo:** o texto que o modelo escreveu é verificado antes de ser
   guardado, mostrado ou escrito no código. Se o modelo inventar ou repetir um
   segredo, é apanhado aqui.

## O que é detetado

| Tipo | Exemplos |
|------|----------|
| `private-key` | Blocos `-----BEGIN ... PRIVATE KEY-----` |
| `aws-key` | Chaves de acesso AWS (`AKIA...`) |
| `jwt` | Tokens JWT |
| `api-token` | Tokens do GitHub, Slack, Stripe, Google |
| `connection-string` | `jdbc:mysql://utilizador:password@servidor/bd`, MongoDB, PostgreSQL, Redis, etc. com credenciais |
| `generic-secret` | Atribuições como `dbPassword = "..."`, `apiKey: "..."`, tokens `Bearer ...` |
| `high-entropy` | Textos longos com ar de aleatórios (parecem chaves), mesmo sem padrão conhecido |

Para evitar falsos alarmes, **não** são tratados como segredos: UUIDs, URLs normais,
caminhos de ficheiros, nomes de classes/propriedades, e valores óbvios de exemplo
(`${DB_PASSWORD}`, `changeme`, constantes em MAIÚSCULAS, etc.).

> A deteção é feita por padrões e estatística. Pode haver falsos alarmes e,
> raramente, segredos que passam sem ser notados. Não substitui boas práticas
> (não guardar credenciais no código).

## A pergunta

Quando encontra algo, pára e mostra o local e o valor **mascarado** (nunca completo):

```
[!] Potential sensitive information detected (connection-string)
    where: prompt for class com.empresa.config.Base (C:\...\Base.java:8)
    value: jdb***db (50 chars)
    [r] redact and continue  [s] skip this snippet  [a] allow once (include as-is)
    [w] not a secret: include it and never ask again for this value  [q] abort run
    choice [r]:
```

| Tecla | Nome | O que acontece |
|-------|------|----------------|
| `r` ou Enter | Esconder (*redact*) | O valor é substituído por `[REDACTED:tipo]` e a geração continua. **É a opção mais segura e a predefinida.** |
| `s` | Saltar | Esta classe ou método é omitido, e fica marcado como `(documentation omitted: sensitive content)`. O modelo não é chamado para ele. |
| `a` | Permitir esta vez | O valor é incluído tal como está, só nesta ocorrência. A decisão fica registada. |
| `w` | Não é um segredo | Como `a`, mas o valor fica na *allowlist* e **não voltas a ser perguntado**. |
| `q` | Abortar | A execução termina. |

Se o mesmo valor aparece várias vezes no mesmo texto, só perguntas uma vez.

## Falsos alarmes: a allowlist

Se o docsmaker marca algo que **não** é um segredo (um identificador de
compilação, um token de exemplo da documentação...), responde `w`. A partir daí
esse valor passa sempre, sem perguntas.

- Guarda-se apenas um **hash** do valor, nunca o valor, em
  `<projeto>/.docsmaker/allowlist.json`, juntamente com o sítio onde foi visto e a data.
- Vale em **todas** as políticas, incluindo `--secrets strict` e `--yes`. É o que
  permite usar a ferramenta em scripts sem ficar bloqueado para sempre por um
  falso alarme.
- Para rever ou desfazer: `docsmaker allowlist list` e `docsmaker allowlist remove <id>`.
- Para um tipo de valor (por exemplo, todos os `exemplo-xxxx`):
  `docsmaker allowlist add-pattern "exemplo-[a-z]{4,}"`.

> **O ficheiro `allowlist.json` é local e não deve ser enviado para o repositório.**
> Um hash de um valor fraco (uma password curta) pode ser descoberto por tentativas.
> O `docsmaker init` já exclui a pasta `.docsmaker/` do git.

## Execuções automáticas (sem ninguém para responder)

| Situação | O que acontece |
|----------|----------------|
| `--yes` ou `--secrets strict` e há um achado **fora** da allowlist | A execução pára com um erro que indica o ficheiro e a linha (valor mascarado). Nunca se interpreta "sem resposta" como "permitido". |
| Não há terminal (entrada fechada) e é preciso perguntar | Erro claro; nada é assumido. |
| Achado **dentro** da allowlist | Passa em silêncio. |
| `--secrets off` | Nenhuma verificação. Aparece um aviso. |

## O registo (auditoria)

Cada decisão fica numa linha de `<projeto>/.docsmaker/audit.log`:

```
2026-11-05T20:33:40Z decision=redact kind=connection-string where="prompt for class ..." value="jdb***db (50 chars)"
```

As decisões possíveis são `redact`, `skip`, `allow-once`, `allow-always`,
`allowlisted` e `abort`. O valor aparece **sempre mascarado**.

## O que fazer em vez de usar a allowlist

Se for uma credencial verdadeira, o melhor é tirá-la do código (variáveis de
ambiente, um cofre de segredos) e, se já esteve num commit, rodá-la. O docsmaker
ajuda a descobrir estes casos, mas a correção é no código.
