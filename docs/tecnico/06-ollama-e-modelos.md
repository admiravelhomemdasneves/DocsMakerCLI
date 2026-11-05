# 6. Ollama e modelos

Código: `internal/llm/` (`ollama.go`, `bootstrap.go`, `local.go`, `detach_*.go`),
`internal/app/model.go`, `internal/cli/model.go`.

## Porque Ollama

- Um único serviço local, com API HTTP simples e gestão de modelos incluída.
- O utilizador não precisa de compilar nada nem de gerir pesos de modelos.
- Está encapsulado atrás de `llm.Client`: trocar de runtime local não toca no pipeline.

O modelo por omissão é `qwen2.5-coder:7b` (`llm.DefaultModel`), um modelo de código de
tamanho razoável para correr em portáteis. Pode mudar-se com `--model`.

## API usada

| Endpoint | Para quê | Timeout |
|----------|----------|---------|
| `POST /api/generate` (`stream:false`) | Gerar texto (`OllamaClient.Generate`) | 5 min por pedido |
| `GET /api/version` | Saber se está a correr e a versão | 10 s |
| `GET /api/tags` | Listar modelos / `HasModel` | 10 s |
| `POST /api/pull` (`stream:true`, NDJSON) | Descarregar um modelo, com progresso | sem limite global |

`HasModel` trata um nome sem *tag* como `:latest`, como o próprio Ollama.

`Pull` lê as linhas `{status, digest, total, completed}`; uma linha `{error}` é um erro, e
um fluxo que termine **sem** `status: success` (interrompido) também é erro.

## `Manager` (bootstrap)

Todas as dependências do sistema são **injetáveis** (`LookPath`, `StartServe`, `Exec`, `GOOS`,
os clientes HTTP), o que permite testar tudo sem instalar nada.

| Método | Faz |
|--------|-----|
| `Status` | `Installed` (executável encontrado **ou** algo a responder), `Running`, versão |
| `PlanInstall` / `Install` | Windows: `winget install --id Ollama.Ollama -e`; macOS: `brew install ollama`; Linux: **manual** |
| `EnsureRunning` | Se não responde e está instalado, lança `ollama serve` separado do processo e espera até 30 s |
| `List` / `HasModel` | Modelos locais |
| `Pull` | Descarrega com callback de progresso |

Pormenores:

- **Localizar o executável:** `PATH` e, depois, os sítios dos instaladores oficiais (porque o
  `PATH` de um terminal aberto antes da instalação não se atualiza): Windows
  `%LOCALAPPDATA%\Programs\Ollama\ollama.exe`; macOS `/Applications/Ollama.app/…`, Homebrew;
  Linux `/usr/local/bin`, `/usr/bin`.
- **Instalação sem aceitar contratos por ti:** o `winget`/`brew` corre ligado ao terminal
  do utilizador (stdin/stdout/stderr), sem `--accept-*-agreements`: se pedir para aceitar
  condições, é a pessoa que responde.
- **`StartServe` separado do processo:** Windows `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP`
  e janela escondida; Unix `Setsid`. O servidor sobrevive ao docsmaker.

## Regras que impedem o código de sair da máquina

Ver [Segurança §4.1](04-seguranca-e-segredos.md): `ResolveBaseURL` (só loopback) e
`CheckLocalModel` (sem modelos *cloud*). Ambas são chamadas por `app` antes de criar o cliente.

## Fluxos de `app/model.go`

```
ModelPull(opts):
  CheckLocalModel(modelo)
  se !Installed:  PlanInstall → (sem comando? erro com instruções) → Confirm → Install → re-Status
  EnsureRunning
  se HasModel:    já disponível
  senão:          Confirm("descarregar do registo…") → Pull(progress)

ensureLocalModel (pré-voo do `doc`):
  CheckLocalModel; EnsureRunning (só arranca, nunca instala); HasModel? senão erro com o comando
```

`Confirm` é injetado pelo driver: `nil` ⇒ recusa (`ErrDeclined`); no CLI, `--yes` responde
sempre sim e, caso contrário, pergunta `Proceed? [y/N]` (só `y`/`yes`).

## Testes

`internal/llm/bootstrap_test.go` usa um servidor HTTP falso e executores falsos:
estado, listagem e `HasModel`, `Pull` (progresso, erro, fluxo cortado, recusa de modelos *cloud*
sem contactar o servidor), `EnsureRunning`, `PlanInstall` por plataforma e `ResolveBaseURL`.
Os testes de `internal/cli` usam o mesmo estilo de Ollama falso (via `OLLAMA_HOST`).

**Não testado a sério:** a instalação com `winget`/`brew` e um download completo de um
modelo real (só com doubles e erros reais do registo).
