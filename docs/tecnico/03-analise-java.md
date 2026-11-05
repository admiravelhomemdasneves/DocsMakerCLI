# 3. Análise de Java

Código em `internal/lang/java/` (adaptador) e `internal/langmodel/` (modelo
intermédio e porto). Descoberta de ficheiros em `internal/project/`.

## Modelo intermédio (`langmodel.CodeModel`)

Tudo o que está para lá da análise (docgen, segredos, inserter) lê **apenas** o
`CodeModel`; nada fora de `lang/*` conhece tree-sitter nem sintaxe Java.

```
CodeModel
 ├─ Language
 ├─ Packages[] ── Package{Name, Types[]}
 │                  └─ TypeDecl{Kind, Name, Package, Extends[], Implements[],
 │                              Fields[], Methods[], ExistingDoc, SourceRange}
 │                       ├─ Field{Name, Type, Modifiers, SourceRange}
 │                       └─ Method{Name, Params[], ReturnType, Modifiers,
 │                                 CallsOut[], ExistingDoc, SourceRange}
 └─ Relationships[]  ── {From, To, Kind: extends|implements|hasField}   (só estes são gerados)
```

- `TypeRef{Name, Package, External}`: `External` significa "fora do projeto" (JDK,
  bibliotecas, ou não resolvido).
- `Range{File, StartByte, EndByte, StartLine, EndLine}`: offsets em **bytes**, para
  o inserter poder editar sem voltar a analisar. O intervalo de uma declaração
  **inclui modificadores e anotações** (começa na primeira anotação).
- `CallRef{MethodName, Receiver, Resolved}`: `Resolved` só é verdadeiro quando o
  receptor e o método foram ambos encontrados no projeto.
- Todos os tipos têm *tags* JSON; `TypeKind` e `RelKind` serializam-se como
  strings. É o mesmo JSON de `docsmaker analyze --format json`.

## Descoberta de ficheiros (`project.DiscoverFiles`)

- Percorre a árvore a partir do caminho dado, filtrando pelas extensões do analisador.
- Salta sempre `.git`, `target`, `build`, `out`, `node_modules` e `.docsmaker`.
- Respeita o `.gitignore` do projeto (`go-gitignore`).
- `DetectBuildSystem` lê `pom.xml` (`<name>` ou `<artifactId>`) ou deteta
  `build.gradle[.kts]`; em último caso usa o nome da pasta. Só serve para dar nome
  ao README (não interpreta dependências nem módulos).

## Duas passagens

### Passagem 1 — extração por ficheiro (`symbols.go`)

Para cada ficheiro: ler → tree-sitter → percorrer os nós de topo.
Para cada declaração de tipo (`class`, `interface`, `enum`, `record`,
`@interface`) extrai o nome, a superclasse e interfaces (em nomes crus), campos,
métodos e construtores com parâmetros e tipo de retorno, modificadores, o
comentário `/** */` imediatamente anterior (`ExistingDoc`) e as chamadas a
métodos feitas no corpo (`method_invocation`).

- Ficheiro que não se consegue analisar → `Diagnostic` de aviso e é ignorado; um
  ficheiro com erros de sintaxe → aviso "syntax errors in file; extraction may be
  incomplete", mas continua com o que for possível.
- Os nomes de campos da gramática (`superclass`, `interfaces`, `declarator`, …)
  foram validados com testes; a função `childByFieldOrType` tolera variações entre
  versões da gramática.

### Passagem 2 — resolução no projeto inteiro (`resolve.go`)

Só é possível depois de se conhecerem **todos** os ficheiros:

1. Constrói a tabela de símbolos (nome simples → tipos do projeto, por pacote).
2. Resolve `extends`, `implements`, tipos de campos, parâmetros e retornos pelo
   **nome simples**: primeiro no mesmo pacote, depois em qualquer pacote do
   projeto; se não encontra, fica `External`.
3. Resolve chamadas: só as `this.x()`/chamadas sem qualificador (métodos do
   próprio tipo) e as feitas sobre um identificador simples que seja **campo**
   do tipo dono. Uma chamada fica `Resolved` se o tipo recetor declara um método
   com esse **nome** (os parâmetros não são comparados).
4. Gera as `Relationships`: `extends` (superclasse, ou interfaces de uma interface), `implements` e
   `hasField` (só quando o tipo do campo é do projeto). Os valores `uses` e `calls` existem no enum
   `RelKind` mas **não são produzidos**; as chamadas ficam em `Method.CallsOut`.

Ordem de pacotes e tipos: a de descoberta de ficheiros (determinística, o que
importa porque alimenta chaves de cache).

## O que **não** é feito (de propósito, para o âmbito do MVP)

| Tema | Comportamento |
|------|---------------|
| Classes aninhadas / internas | Não são percorridas (só tipos de topo) |
| Genéricos | `Map<String, List<X>>` reduz-se a `Map`; sem inferência |
| `import`s | Não se usam para resolver; só o nome simples e o pacote |
| Nomes iguais em pacotes diferentes | Resolve-se para o do mesmo pacote; se não, o primeiro encontrado |
| Sobrecarga e chamadas encadeadas/estáticas | Sem resolução (a chamada fica `Resolved=false`) |
| Lambdas, classes anónimas | Sem tratamento específico |
| Módulos Maven/Gradle | Tratados como uma só árvore de ficheiros |

Para a documentação isto chega (descrever responsabilidade e relações), mas
convém conhecê-lo: uma relação em falta no `analyze` não significa que o código
não a tem.

## Testes

`internal/lang/java/analyzer_test.go` cobre a extração de tipos, campos e métodos,
`extends`/`implements`/chamadas resolvidas entre pacotes, e um ficheiro ilegível a ser
ignorado como diagnóstico; `internal/project/fileset_test.go` cobre a descoberta de
ficheiros (`.gitignore`) e a deteção Maven. O projeto de exemplo
em `testdata/sample-project` (pacotes `greeting`, `animal`, `config`) serve
também os testes do CLI e a demo.
