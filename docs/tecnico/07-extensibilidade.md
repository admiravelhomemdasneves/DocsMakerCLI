# 7. Extensibilidade

Receitas para as extensões previstas. Em todas, o núcleo não muda: acrescenta-se um
adaptador atrás de um porto existente.

## 7.1 Adicionar uma linguagem

Exemplo: Kotlin (comentários `/** */`, como o Java).

1. **Analisador** em `internal/lang/kotlin/`, a implementar `langmodel.LanguageAnalyzer`:
   - `Language()` ⇒ `"kotlin"`; `FileExtensions()` ⇒ `[".kt"]`;
   - `ParseProject` devolve um `CodeModel` com `Range` em **bytes** e o início da declaração
     **incluindo anotações/modificadores** (o inserter depende disto);
   - ficheiros ilegíveis ⇒ `Diagnostic`, nunca erro fatal;
   - para o verificador do modo inline (`app.parseVerifier`), um diagnóstico conta como "ficheiro
     partido" se a mensagem contiver `syntax error` **ou** se não for um aviso; usa estas convenções
     para assinalar erros de sintaxe.
2. **Registo:** `init()` do pacote chama `langmodel.Register("kotlin", factory)`; e
   `cmd/docsmaker/main.go` importa o pacote com `_`.
3. **Efeitos automáticos:** `init` passa a acrescentar `*.kt filter=docsmaker-ai-doc`
   (itera as linguagens registadas); `analyze`/`doc` usam o analisador certo.
4. **A rever à mão** (hoje assumem Java):
   - `app.resolveAnalyzer`: com mais de uma linguagem registada, a escolha automática dá erro
     e pede `--language`; a *flag* ainda **não existe** no CLI (o campo `Language` já está em
     `AnalyzeOptions`/`GenerateDocsOptions`). Para um projeto misto é preciso expô-la, ou
     decidir pela extensão dominante;
   - `internal/llm/prompts`: os *system prompts* dizem "Java"; parametrizar pela linguagem;
   - `inserter.BlockCommentInserter`: serve qualquer linguagem de comentários `/** */`; outra
     sintaxe (`#`, `"""`) exige outra implementação de `Inserter`; `sanitize` também tem
     regras específicas de Java (`\u`);
   - `secrets`: os detetores são independentes da linguagem; o de atribuição por identificador
     assume sintaxe tipo C/Java (`nome = "valor"`, `"chave": "valor"`).
5. **Testes:** copiar o padrão de `lang/java/analyzer_test.go`; e um projeto de exemplo em `testdata/`.

## 7.2 Adicionar um driver (uma UI, ou `docsmaker serve`)

`internal/app` já é a fronteira: funções que recebem *options* e devolvem *structs*
(com *tags* JSON). Um driver novo:

- **Embutir:** importar `internal/app` e chamar `Analyze`, `GenerateDocs`, `Init`, `ModelStatus`,
  `ModelList`, `ModelPull`, `Allowlist*`.
- **Servidor local:** um comando `docsmaker serve` que expõe estas funções em HTTP **só em
  loopback** (mantém a garantia de não sair da máquina). O JSON de `analyze --format json` e
  de `doc --format json` já é o contrato natural.

Pontos de integração que uma UI tem de fornecer:

| Necessidade | Porto / campo |
|-------------|---------------|
| Perguntar o que fazer a um achado sensível | Implementar `secrets.Approver` (um modal em vez de `TerminalApprover`) e passar em `GenerateDocsOptions.Approver` |
| Confirmar instalações e downloads | `ModelPullOptions.Confirm` |
| Mostrar progresso / notas | `ModelPullOptions.Progress`/`Log`, `GenerateDocsOptions.Logf` |
| Cancelar | Todos os casos de uso recebem `context.Context` |

Limitação: `GenerateDocs` é uma chamada longa e bloqueante, sem eventos de progresso por unidade.
Uma UI com barra de progresso exigiria acrescentar um callback ao pipeline.

## 7.3 Adicionar um detetor de segredos

Em `internal/secrets/detectors.go`, acrescentar um `detector{kind, re, useGroups, keep}` à lista
`detectors`. **A ordem é a prioridade** (as regras mais específicas primeiro); em sobreposição,
vence a anterior. Usar `useGroups` para marcar só o valor (para que a redação preserve a chave) e
`keep` para vetar placeholders. Acrescentar um caso positivo **e** um negativo em `detectors_test.go`.
Se for um tipo novo, definir a constante `Kind` em `scanner.go`.

## 7.4 Trocar a forma de decidir sobre segredos

`secrets.Approver` é a única coisa que decide. Para uma política corporativa (por exemplo, aprovar
automaticamente certos tipos, ou consultar um serviço interno), implementar `Approver` e
injetá-lo em `GenerateDocsOptions.Approver`. O `Gate` regista tudo na auditoria.

## 7.5 Novas formas de saída

- **Ficheiros à parte:** `output.Writer` (`SidecarWriter` é o exemplo).
- **Nos fontes:** `inserter.Inserter` (`BlockCommentInserter`).
- Nota: o modo `console` não passa pelo `Writer`; é renderizado em `internal/cli` a partir do resultado.
  Um novo formato (por exemplo, Markdown consolidado) é um novo *render* no driver ou um novo `Writer`
  ligado em `GenerateDocs`.

## 7.6 Outro runtime de modelo local

Implementar `llm.Client` (`Generate`, `Available`) e escolher o cliente em `GenerateDocs`. **Regra
do projeto:** nenhum cliente pode falar com um serviço remoto. A garantia é estrutural, não uma flag.
Se o novo runtime tiver o seu próprio bootstrap, ele entra ao lado de `llm.Manager`.

## 7.7 Depois de qualquer extensão

- `gofmt`, `go vet ./...`, `go test ./...`;
- correr `scripts/demo.sh` (confirma que as garantias de ponta a ponta se mantêm);
- atualizar a documentação em `docs/`.
