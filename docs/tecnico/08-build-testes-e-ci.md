# 8. Build, testes e CI

## Requisitos de desenvolvimento

- **Go 1.26+** (ver `go.mod`).
- **Um compilador de C** (gcc ou clang): a gramática tree-sitter é código C, logo o build usa **CGo**
  (`CGO_ENABLED=1`).
- `git` (usado em tempo de execução pelo `init`/`inline` e nos testes que criam repositórios; esses
  testes fazem *skip* se o git não existir).
- Para a demo: um Ollama local com um modelo, e `bash`.

### Windows

1. `winget install GoLang.Go`
2. Um MinGW-w64 (por exemplo WinLibs: `winget install BrechtSanders.WinLibs.POSIX.UCRT`).
3. **O compilador tem de estar num caminho sem espaços** (por exemplo `C:\mingw64`). Se estiver num
   caminho como `C:\Users\Nome Apelido\…`, o linker do MinGW falha com
   `linker script file … appears multiple times`. Copiar a pasta do toolchain para `C:\mingw64` resolve.
4. Para compilar à mão:

```
set CGO_ENABLED=1
set CC=C:\mingw64\bin\gcc.exe
set PATH=C:\Program Files\Go\bin;C:\mingw64\bin;%PATH%
go build -o docsmaker.exe ./cmd/docsmaker
```

### Não há *cross-compile*

Com CGo, `GOOS=linux go build` num Windows falha (faltam cabeçalhos e libs). Por isso o CI compila
**nativamente em cada sistema operativo**.

## Estrutura do repositório

```
cmd/docsmaker/        main
internal/             todo o código (ver 01-arquitetura.md)
testdata/sample-project/   projeto Java de exemplo (com uma credencial falsa plantada)
scripts/              build.sh, build.ps1, install.sh, install.ps1, demo.sh
.github/workflows/    ci.yml, release.yml
docs/                 esta documentação
README.md             resumo em inglês
```

## Testes

```
go vet ./...
go test ./...
```

| Camada | O que cobre | Notas |
|--------|-------------|-------|
| Unitários por pacote | Analisador, resolução, descoberta de ficheiros, cache, pipeline, deteção/redação/gate/allowlist, `Clean`, `Setup`, inserter, Manager/`ResolveBaseURL` | Rápidos; sem rede |
| `internal/docgen/gate_test.go` | Os dois portões no pipeline, com um cliente que regista prompts | |
| `internal/cli/cli_test.go` | Toda a linha de comandos em processo, contra um **Ollama falso** local | Define `OLLAMA_HOST` para um `httptest.Server` (loopback, por isso aceite) |
| `scripts/demo.sh` | De ponta a ponta: Ollama **real**, repositório git descartável, commits reais | Falha (código ≠ 0) se uma garantia se quebrar |

Cobertura aproximada (instruções): `cli` ≈ 91 %, `lang/java` ≈ 89 %, `inserter` ≈ 87 %, `secrets` ≈ 87 %,
`docgen` ≈ 83 %, `gitfilter` ≈ 77 %, `llm` ≈ 62 %. `app` mede pouco (≈ 17 %) porque é coberto
sobretudo pelos testes do CLI e pela demo. Falta cobrir o arranque real de processos e as instalações
com `winget`/`brew`.

Os testes de CLI foram **validados por mutação** (estragar a allowlist ou a redação faz falhar o teste certo).

### A demo (`scripts/demo.sh`)

```
scripts/demo.sh [caminho/para/docsmaker]
DEMO_MODEL=…  DEMO_PAUSE=1  DEMO_OFFLINE=1  DEMO_KEEP=1
```

Cria um repositório temporário a partir de `testdata/sample-project` e verifica, entre outras coisas:
`inline` recusado antes do `init`; `--yes` bloqueia a credencial plantada; o segredo não aparece em nenhum
comentário, saída, cache nem auditoria; `git diff`/commit sem linhas de IA, mesmo depois de uma edição
humana no mesmo ficheiro; segunda execução idempotente; `OLLAMA_HOST` remoto e modelos *cloud* recusados;
só `internal/llm` usa `net/http`; allowlist (`w`, `--yes`, `remove`).
`DEMO_OFFLINE=1` acrescenta um passo em que **tu** desligas a rede (nunca foi executado).

## Build e empacotamento

```
VERSION=v1.0.0 scripts/build.sh      # Linux / macOS (e Git Bash)
$env:VERSION="v1.0.0"; scripts\build.ps1   # Windows
```

Cada script: compila com `-trimpath -tags netgo,osusergo -ldflags "-s -w -X …/internal/cli.Version=$VERSION"`
(no Linux, ligação estática), **executa o binário** (`--version`) como teste de fumo, junta
`README.md` e o instalador, cria `dist/docsmaker_<versão>_<so>_<arq>.{zip|tar.gz}` e o `.sha256`.

Resultado no Windows: ≈ 8 MB, depende só de DLLs do sistema (UCRT); sem DLLs do MinGW.

A versão por omissão é `git describe --tags --always --dirty`, ou `dev` se não for um repositório git.

## Integração contínua (GitHub Actions)

| Ficheiro | Quando | O que faz |
|----------|--------|-----------|
| `.github/workflows/ci.yml` | *push* para `main` e *pull requests* | Em Ubuntu, Windows e macOS: `gofmt` (só no Linux), `go vet`, `go test`, e o *build* completo do pacote como teste de fumo |
| `.github/workflows/release.yml` | *tag* `v*` | Em cada SO: testa, constrói e empacota; publica os pacotes e checksums numa *release* (`gh release create --generate-notes`) |

Para lançar uma versão: `git tag v1.0.0 && git push origin v1.0.0`.

**Estado de validação:** estes *workflows* foram escritos mas **nunca executados**. Os *scripts* de build foram
exercitados no Windows (e o `build.sh` no Git Bash); a ligação estática no Linux e o build em macOS só ficam
validados na primeira execução real. Se o repositório não for GitHub, os *scripts* em `scripts/` são a fonte da
verdade e os *workflows* são finos; portar para outra plataforma é reescrever só os YAML.

## Distribuição e assinatura

Os binários **não estão assinados**. Windows SmartScreen e macOS Gatekeeper podem avisar. Se a organização o
exigir: assinatura Authenticode (Windows) e *notarization* (macOS) acrescentam-se ao passo de empacotamento.

## Convenções

- `gofmt` obrigatório (o CI falha se `gofmt -l .` listar ficheiros).
- Erros: `fmt.Errorf("pacote: contexto: %w", err)`; mensagens ao utilizador dizem o que fazer a seguir.
- Nada de lógica no `internal/cli`.
- Qualquer comportamento que proteja o utilizador (segredos, git, rede local) tem um teste que **falha** se for removido.
