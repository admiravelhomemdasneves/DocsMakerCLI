# 9. Decisões e limitações

## 9.1 Decisões de desenho (e porquê)

| # | Decisão | Alternativas consideradas | Razão |
|---|---------|---------------------------|-------|
| 1 | **Go**, executável único nativo | Java + GraalVM native-image; Rust; Python empacotado | Um binário pequeno, rápido a arrancar, instalação trivial e sem runtime. Foi pedido "a linguagem mais apropriada"; o Java com GraalVM complicaria o uso de bibliotecas nativas (tree-sitter) e o build |
| 2 | **tree-sitter** para analisar | Parser Java em Java (JavaParser); regex | Gramáticas para muitas linguagens com a mesma API ⇒ extensível; tolera código com erros |
| 3 | **Ollama** como único motor | llama.cpp embutido; API remota | Gestão de modelos e API prontas; local por natureza. Embutir llama.cpp aumentaria muito a complexidade de build |
| 4 | **Arquitetura hexagonal** com `internal/app` como fronteira | CLI monolítico | Permite uma UI/`serve` futuros sem tocar no núcleo; testabilidade (Ollama falso, doubles) |
| 5 | **Sumarização hierárquica** (método → classe → pacote → projeto) | Enviar o ficheiro/projeto inteiro | Janelas de contexto pequenas; cada nível é cacheado independentemente |
| 6 | **Cache por hash do pedido exato** | Cache por ficheiro/data | Invalida sozinha quando muda o código, o prompt ou o modelo; sem controlo de frescura |
| 7 | **Dois portões** de segredos (antes e depois do modelo) | Só antes | O modelo pode repetir/inventar um segredo; o segundo portão corre também nos acertos de cache |
| 8 | **`--yes` bloqueia** em vez de permitir | Permitir por omissão | "Sem resposta" nunca deve significar "permitido" |
| 9 | **Allowlist por hash, local**, também válida em `strict`/`--yes` | Allowlist versionada com valores; sem allowlist | Um hash de valor fraco é adivinhável ⇒ não partilhar; sem ela um falso positivo bloquearia a automação |
| 10 | **Filtro git `clean`** com marcas dentro dos comentários | Ficheiros à parte (sidecar) como único modo; `git update-index --skip-worktree`; hooks | Cumpre "uma linha de IA numa classe que vai ao repositório" ao nível da linha; testado com edições humanas no mesmo ficheiro (outras formas de `git add`, como `-p`, não foram testadas) |
| 11 | **Atributos em `.git/info/attributes`** por omissão | `.gitattributes` versionado | Não altera ficheiros versionados nem chega ao repositório central; `--shared` existe para quem quiser partilhar |
| 12 | **Só `clean`, `required=true`** | `smudge` também; filtro opcional | `smudge` seria a identidade; `required` faz o commit falhar se o binário faltar em vez de vazar linhas |
| 13 | **Recusar** `OLLAMA_HOST` remoto e modelos *cloud* | Avisar apenas | A garantia central é "o código não sai da máquina": não pode depender do ambiente do utilizador |
| 14 | **Linux: sem instalação automática** do Ollama | `curl … \| sh` | Não executar scripts descarregados da rede |
| 15 | **Nunca tocar em comentários escritos por pessoas** | Substituir/aumentar | Segurança e respeito pelo trabalho existente |
| 16 | **Verificar por re-análise** cada ficheiro escrito, escrita atómica | Confiar na inserção textual | Um bug de inserção nunca deixa um ficheiro partido no disco |
| 17 | **Pedidos sequenciais** ao modelo | Paralelismo | Um modelo local satura o hardware; simplicidade e determinismo |
| 18 | **Resolução de tipos por nome simples** | Resolução completa com imports/classpath | Suficiente para documentação; um compilador completo está fora do âmbito |
| 19 | **Extração de regras como comando próprio, sem modo inline** | `doc --level rules`; bloco `@businessRule` no Javadoc | Saída de outra natureza (regra + evidência + confiança) e uma hipótese de IA dentro do código tem mais risco do que num relatório. Detalhes e decisões em [10](10-extracao-de-regras.md) |
| 20 | **Confiança limitada pela evidência verificada no código** | Confiar na confiança declarada pelo modelo | Modelos pequenos declaram "alta" com facilidade; uma regra sem linhas reais que a sustentem nunca passa de `low` |
| 21 | **Respostas inutilizáveis nunca vão para a cache** | Guardar tudo | Um JSON mal formado em cache seria devolvido, igual, em todas as execuções |

## 9.2 Limitações conhecidas

### Funcionais
- **Só Java**, e só tipos de topo (sem classes aninhadas), sem resolução por `import`, genéricos reduzidos ao
  tipo base, chamadas resolvidas só para `this`/sem qualificador e campos do tipo dono.
- **Sobrecarga de métodos:** os resumos partilham o nome; o prompt da classe mostra o último; `--target …#nome`
  escolhe o primeiro. (O modo inline documenta cada sobrecarga corretamente.)
- **O README do projeto não é escrito em `README.md`:** é gerado (consola / `--mode sidecar`) mas copiar para o
  projeto é manual. Escrever/atualizar o `README.md` do projeto é um passo natural a acrescentar.
- **Sem `--language`** no CLI (o campo existe no `app`); irrelevante enquanto só houver Java.
- **Qualidade** limitada pelo modelo local; os resultados com modelos pequenos (1.5B) são genéricos.
  Os testes automáticos não avaliam a qualidade do texto.
- **Sem retentativas nem streaming** no `doc`. O `doc` também não tem progresso por unidade (o `rules` mostra-o no stderr).
- **Regras de negócio** (`rules`): o "porquê" é sempre uma inferência do modelo; com o modelo de 1.5B medido,
  9 de 15 regras plantadas saem com todos os valores certos e 5 de 6 métodos técnicos são tomados por regras;
  cada método é analisado isoladamente. Lista completa em
  [10 — Limitações](10-extracao-de-regras.md#9-limitações).

### Git e inline
- Os blocos de IA vivem só no *working tree*: um `pull` que altere o ficheiro, ou `stash`, apaga-os (re-gerar é
  rápido, vem da cache).
- Cada pessoa tem de correr `docsmaker init` no seu clone; mudar o executável de sítio exige repetir.
- Um `/** */` vazio escrito por uma pessoa é removido do commit.
- `core.autocrlf=true` não foi testado.
- Só `.java`/UTF-8; ficheiros noutras codificações são ignorados.

### Segurança
- Deteção heurística (falsos positivos e negativos possíveis). Não substitui um *secret scanner* dedicado.
- A cache e os resultados em `--mode sidecar` são texto em claro no disco, fora do git.
- Confia-se no Ollama como programa; não é uma *sandbox* de rede.

### Distribuição e validação
- **Só o Windows foi construído e testado.** Linux e macOS dependem da primeira execução do CI.
- Os *workflows* do GitHub Actions nunca foram executados.
- Binários **não assinados**.
- O `install.ps1` só foi verificado sintaticamente (altera o `PATH` do utilizador, por isso não foi corrido
  durante o desenvolvimento).
- A instalação do Ollama com `winget`/`brew` e um download completo de modelo só foram testados com *doubles*.
- O passo opcional `DEMO_OFFLINE=1` (rede desligada) nunca foi executado.

## 9.3 Ideias para a evolução

Ordenadas por valor/esforço (opinião, não compromisso):

1. Escrever o README gerado em `README.md` do projeto (com *diff* e confirmação).
2. `--language` e suporte a uma segunda linguagem (Kotlin ou TypeScript) para provar a extensibilidade.
3. Parametrizar os prompts por linguagem e permitir *prompts* configuráveis por projeto.
4. Eventos de progresso no pipeline (para barra de progresso e UI).
5. `docsmaker serve` (HTTP em loopback) e uma UI simples para quem tem menos experiência com a linha de comandos.
6. Assinatura dos binários e *package managers* (winget, Homebrew, scoop).
7. Resolução de `import`s e classes aninhadas na análise de Java.
8. Detetores de segredos configuráveis por projeto (ficheiro versionado de *padrões*, sem valores).
9. Evolução do `rules`: avaliação de qualidade com vários modelos, histórico git como pista do "porquê",
   consolidação ao nível do projeto (ver [10 — Ideias seguintes](10-extracao-de-regras.md#10-ideias-seguintes)).
