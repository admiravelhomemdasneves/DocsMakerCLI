# 5. Inserção e filtro git

Objetivo: o programador vê a documentação gerada **nos próprios ficheiros**,
mas as linhas escritas pela IA **nunca entram num commit**, nem sequer em
ficheiros que contêm também alterações humanas.

Código: `internal/inserter` (escrever), `internal/gitfilter` (limpar e configurar),
`internal/app/inline.go` (orquestrar e verificar).

## O contrato das marcas

```java
    /**
     * docsmaker:ai-begin:v1
     * Texto gerado.
     * docsmaker:ai-end
     */
    public void metodo() { }
```

- `ai-begin:v<N>` — o número de versão permite que versões futuras do filtro mudem
  o formato sem partir ficheiros marcados por versões antigas (`CurrentVersion = "v1"`).
- A deteção é por *substring* (regex `docsmaker:ai-begin(:v\d+)?` / `docsmaker:ai-end`),
  linha a linha.

## O filtro `clean` (`gitfilter.Clean`)

Contrato do git: stdin → stdout.

1. Lê tudo e **valida** a profundidade das marcas. Um `ai-end` sem `ai-begin`, ou um
   `ai-begin` sem `ai-end`, é **entrada malformada**: devolve o ficheiro **intacto** e
   avisa no stderr (preferível a corromper o que vai para o commit).
2. Remove as linhas desde `ai-begin` até `ai-end`, inclusive.
3. Colapsa os comentários `/** */` que ficaram vazios (só com `*` de continuação)
   incluindo a quebra de linha, para que o resultado seja **exatamente o original**.

`Smudge` é a função identidade e **não é registada** no git (custaria um processo por
checkout sem efeito).

Propriedade-chave, testada: `Clean(Inserir(x)) == x` byte a byte (também com CRLF e com
conteúdo hostil do modelo).

Efeito colateral conhecido: o passo 3 também remove, do commit, um `/** */` **vazio**
escrito por uma pessoa.

## Configuração (`gitfilter.Setup`, comando `init`)

Idempotente. Passos:

1. `git rev-parse --show-toplevel` (falha se não for um repositório).
2. Atributos: acrescenta `<padrão> filter=docsmaker-ai-doc` por cada extensão das
   linguagens registadas (hoje `*.java`) a **`.git/info/attributes`** (local; resolvido com
   `git rev-parse --git-path`, por isso funciona com *worktrees*). Com `--shared`, escreve
   no `.gitattributes`. Se o padrão já tem **outro** `filter=`, falha — a menos que `--force`.
3. Configuração do git (**local**):
   `filter.docsmaker-ai-doc.clean = "<caminho absoluto do docsmaker>" git-clean-filter clean`
   (barras normais, mesmo no Windows, porque o git executa o comando por `sh`) e
   `filter.docsmaker-ai-doc.required = true`.
4. Acrescenta `.docsmaker/` a `.git/info/exclude`.

Decisões (e porquê):

| Decisão | Motivo |
|---------|--------|
| `.git/info/attributes` por omissão | Nenhum ficheiro versionado é alterado; nada do docsmaker chega ao repositório central |
| `required = true` | Se o binário faltar, o `git add` **falha** em vez de deixar passar linhas de IA |
| Caminho absoluto do executável | Funciona sem o docsmaker estar no `PATH`; se mudar de sítio, repetir `init` (o valor é atualizado sem `--force` quando o antigo ainda é "nosso") |
| Só `clean` | `smudge` seria a identidade |

## A verificação antes de escrever (`gitfilter.EnsureWired`)

Corre **antes** de gerar (a geração é lenta) quando `--mode inline`:

- Fora de um repositório git: passa (não há onde fazer commit). Se o `git` não está
  instalado mas existe um `.git` acima: **erro** (não dá para garantir).
- A configuração `filter.docsmaker-ai-doc.clean` tem de existir.
- `git check-attr -z filter` (agrupado por pasta, em lotes de 100) tem de devolver
  `docsmaker-ai-doc` para **todos** os ficheiros descobertos. Ser agrupado por pasta e
  correr *na* pasta evita comparar caminhos (nomes curtos 8.3 no Windows, `..`).
  Cobre repositórios aninhados e *submodules*, que têm a sua própria configuração.

## A inserção (`inserter.BlockCommentInserter`)

Porto: `InsertDocs(ctx, file, []Insertion) ([]Outcome, error)`; atómico por ficheiro.

1. Lê o ficheiro; se não for UTF-8 válido, ignora-o (`skipped`). Deteta CRLF.
2. Ordena as inserções por posição **decrescente**, para os offsets anteriores
   continuarem válidos.
3. Para cada uma (`applyOne`):
   - o offset tem de estar dentro do ficheiro e a declaração tem de começar numa linha
     só com indentação (senão `skipped`);
   - olha para o que está imediatamente acima: se for um `/** … */`:
     - **com** marcas de IA → substitui (`replaced`, ou `unchanged` se igual);
     - **sem** marcas → é de uma pessoa: `skipped` ("has a hand-written doc comment");
   - caso contrário, insere (`inserted`) com a indentação da declaração.
4. Se algo mudou e há `Verify`, chama-o com o original e o resultado.
5. Escreve para um ficheiro temporário **na mesma pasta** e `rename` (atómico),
   preservando as permissões.

### Sanitização do texto do modelo (`sanitize`)

| Problema | Tratamento |
|----------|------------|
| `*/` fecharia o comentário | `* /` |
| `/*` confundiria a deteção do comentário anterior | `/ *` |
| `\u` é um escape unicode (**erro de compilação** em Java, mesmo num comentário) | `\\u` |
| `docsmaker:ai-begin/end` forjaria ou fecharia uma marca | `gitfilter.Neutralize` ⇒ `docsmaker: ai-end` |
| Quebras de linha `\r\n`, espaços no fim | Normalizadas |

### Verificação por re-análise (`app.parseVerifier`)

Escreve o original e o resultado em ficheiros temporários, analisa ambos com o mesmo
`LanguageAnalyzer` e exige: sem **novos** erros de sintaxe, e a **mesma impressão digital
de declarações** (tipos, campos, métodos com a contagem de parâmetros). Se falhar, o
ficheiro não é tocado e devolve-se erro. Isto detecta, por exemplo, um comentário que
engolisse código.

## Ciclo de vida e limitações

- Os blocos vivem só no *working tree*. Operações do git que reescrevem o ficheiro
  (`pull` que o altera, `stash`) removem-nos; repetir `docsmaker doc --mode inline` restaura-os
  quase instantaneamente (cache).
- Um `git checkout -- .` de ficheiros que o git considera inalterados (o `clean` torna-os
  iguais ao índice) **não** os reescreve.
- O `git status` pode listar ` M` logo a seguir à inserção, por o índice ter *stat* antigo;
  o `git diff`/`add` já vê o conteúdo limpo e fica vazio.
- `core.autocrlf=true` não foi testado.
- Cada pessoa tem de correr `init` no seu clone; a configuração do filtro não viaja com o repositório.

## Testes

| O quê | Onde |
|-------|------|
| `Clean`: remoção, malformado, colapso de vazios | `gitfilter/clean_test.go` |
| `Setup`/`EnsureWired` com repositórios reais, idempotência, conflito, `--shared` | `gitfilter/setup_test.go` |
| Inserção, ida-e-volta com `Clean`, idempotência, hostil, CRLF, falha de verificação, não UTF-8 | `inserter/blockcomment_test.go` |
| Verificador por re-análise, `Init` | `app/inline_test.go` |
| Fluxo completo e comando do filtro | `cli/cli_test.go` (`TestInitAndInline`) |
| Commits reais | `scripts/demo.sh` |
