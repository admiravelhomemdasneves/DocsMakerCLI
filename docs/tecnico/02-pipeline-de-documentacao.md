# 2. Pipeline de documentação

Implementado em `internal/docgen/pipeline.go`. Os prompts estão em
`internal/llm/prompts/prompts.go` (montagem pura de strings, sem I/O).

## Sumarização hierárquica (de baixo para cima)

Um modelo local tem uma janela de contexto pequena. Em vez de enviar o projeto
todo, cada nível consome os **resumos já escritos** do nível abaixo:

```
método ──► classe ──► pacote ──► projeto (README)
 (código)   (resumos    (resumos     (resumos
             dos         das          dos
             métodos     classes)     pacotes)
             + campos)
```

| Nível | Entrada do prompt | Função |
|-------|-------------------|--------|
| `method` | Classe dona, assinatura, **código-fonte** do método | `summarizeMethod` |
| `class` | Tipo, `extends`/`implements`, **declarações dos campos tal como escritas** (com valores iniciais), assinatura + resumo de cada método | `summarizeClass` |
| `package` | Tipos do pacote com o resumo de cada um, relações entre eles | `summarizePackage` |
| `project` | Nome do projeto, resumo de cada pacote (**ordenados**) | `runProject` |

Cada `Run` devolve **todos** os artefactos consumidos, de baixo para cima (pedir
uma classe devolve os métodos e depois a classe). Cada `Artifact` tem
`Level`, `Target`, `Content` e `Source` (o intervalo de código a que se refere,
usado pelo modo inline).

As declarações de campos entram no prompt da classe porque é onde costumam estar
constantes e credenciais; sem isso, o primeiro portão de segredos nunca as veria.

O `README` final é **`BuildReadme`**: o texto de visão geral vem do modelo, mas a
secção *Structure* (pacotes e tipos) vem diretamente do `CodeModel`, nunca do
modelo. O projeto não escreve este README em disco no `README.md` do projeto
(ver [limitações](09-decisoes-e-limitacoes.md)).

## O ponto único de geração: `generate`

Todos os níveis passam por `Pipeline.generate(ctx, where, system, user)`:

```
1. Portão 1 (prompt):  Gate.Screen("prompt for <where>", user)
        └─ Skip?  → devolve OmittedSummary, sem chamar o modelo, sem cache
2. Cache:              key = sha256(modelo, system, user)   ← o prompt já "limpo"
        └─ hit?   → usa o texto guardado
3. Modelo:             Client.Generate(...)  (só se não houve hit); TrimSpace
4. Portão 2 (saída):   Gate.Screen("generated output for <where>", texto)
        └─ corre também nos hits da cache
5. Cache.Set(key, texto já filtrado)  (só se veio do modelo)
```

Consequências deliberadas:

- A chave da cache é o pedido **exato** enviado ao modelo (modelo + system +
  prompt). Mudar o código, o prompt-template ou o modelo invalida
  automaticamente só o que depende disso; não há controlo de "frescura" separado.
- O prompt entra na chave **depois** da redação; trocar a decisão de um achado
  (por exemplo de `redact` para `allow`) muda o prompt e portanto a chave.
- O portão de saída corre também nos acertos de cache, para uma cache antiga não
  contornar a verificação (por exemplo, depois de a allowlist mudar).
- O que se guarda em cache é o texto **depois** do portão 2.
- `Skip` devolve `OmittedSummary = "(documentation omitted: sensitive content)"`;
  os níveis acima resumem à volta disto.

## Cache

`internal/cache`: `Store{Get,Set}`; `FileStore` guarda um ficheiro
`<chave>.txt` por entrada em `<projeto>/.docsmaker/cache/`. `Key(parts...)`
faz SHA-256 com um byte NUL antes de cada parte (para que `("ab","c")` e
`("a","bc")` não colidam). Não há expiração nem limpeza: apagar a pasta limpa tudo.

## Determinismo

Prompts são chave de cache, por isso têm de ser determinísticos. A ordem dos
pacotes e tipos vem da ordem de descoberta de ficheiros (lista, não `map`), e o
prompt de projeto ordena os pacotes alfabeticamente (`prompts.Project`, com
teste: um `map` iterado diretamente dava um prompt diferente a cada execução e
falhas de cache aleatórias).

## Prompts

Os *system prompts* pedem texto simples (sem markdown nem cercas de código),
resumos curtos (2–4 frases por método, 3–6 por classe, 3–5 por pacote, 1–2
parágrafos para o projeto) e, nos métodos, uma linha `@param` por parâmetro e
`@return`. Estão escritos em inglês e mencionam "Java" explicitamente (ver
[Extensibilidade](07-extensibilidade.md) para o que mudar ao adicionar uma
linguagem).

## Pedidos ao modelo

`OllamaClient.Generate` faz `POST /api/generate` com `stream: false` e
timeout de 5 minutos por pedido. Os pedidos são **sequenciais**: não há
paralelismo (um modelo local satura o hardware de qualquer forma).

## Limitações conhecidas do pipeline

- **Sobrecarga de métodos:** os resumos de métodos são indexados por nome; com
  sobrecargas, o último substitui os anteriores no prompt da classe, e o
  `--target pacote.Classe#nome` escolhe o primeiro. (O modo inline não é
  afetado: usa o intervalo de código de cada método.)
- Sem retentativas nem *backoff* em caso de erro do modelo: o erro interrompe a execução
  (o que já foi gerado fica em cache, por isso repetir continua de onde parou).
- Sem streaming: a saída aparece quando cada pedido termina.
