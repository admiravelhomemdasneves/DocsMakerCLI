# 6. Documentação dentro do código

Com `--mode inline`, o docsmaker escreve a documentação como **comentários
`/** ... */` diretamente nos ficheiros `.java`**, por cima de cada classe e
método. O git está configurado para que esses comentários da IA **nunca**
entrem num commit, mesmo que o ficheiro tenha outras alterações tuas.

## Como funciona (em linguagem simples)

1. Cada bloco escrito pela IA tem duas linhas-marca, `docsmaker:ai-begin` e
   `docsmaker:ai-end`:

   ```java
   /**
    * docsmaker:ai-begin:v1
    * Calcula o total da fatura, somando as linhas e aplicando o desconto.
    * docsmaker:ai-end
    */
   public BigDecimal total() { ... }
   ```

2. Quando fazes `git add` / `git commit`, o git passa cada ficheiro por um filtro
   que **remove** tudo o que está entre essas marcas. O que vai para o commit é
   exatamente o teu código, sem os blocos da IA.
3. No teu disco os blocos **continuam lá**, para os leres e usares no IDE.

Consequência: as tuas alterações normais seguem para o repositório; a
documentação gerada fica só contigo.

## Preparar (uma vez por clone)

```
docsmaker init
```

Corre-o dentro do repositório (ou indica o caminho). O que faz:

- regista, **só neste clone**, a regra "ficheiros `*.java` passam pelo filtro do docsmaker"
  (em `.git/info/attributes` e `.git/config`; **não altera nenhum ficheiro versionado**);
- marca o filtro como obrigatório: se o `docsmaker` desaparecer, o `git add` **falha**
  em vez de deixar passar comentários da IA sem aviso;
- exclui a pasta `.docsmaker/` do git.

Repetir o comando é seguro. **Cada pessoa que use `--mode inline` tem de o correr
uma vez no seu clone**, porque a configuração é local. Se movimentares o programa
para outra pasta, volta a correr `docsmaker init`.

> Se preferires partilhar a regra com a equipa, `docsmaker init --shared` escreve-a
> no `.gitattributes` (versionado). Mesmo assim, cada pessoa continua a precisar do `init`
> no seu clone para configurar o filtro.

## Gerar

```
docsmaker doc --mode inline
```

(ou apontando para uma classe: `--level class --target com.empresa.faturacao.Fatura`).

Se o repositório **não** tiver o `init` feito, o docsmaker **recusa-se a escrever** e
diz-te para o correr. Isto é verificado *antes* de gastar tempo com o modelo.
Fora de um repositório git não há risco de commit, por isso não é exigido.

## O que ele respeita

| Situação | O que acontece |
|----------|----------------|
| Já existe um comentário escrito por uma pessoa em cima do método/classe | **Não é tocado**; aparece como `skipped` |
| Já existe um bloco de IA de uma execução anterior | É **atualizado** no lugar (não duplica) |
| Correr duas vezes seguidas | Segunda vez: tudo `unchanged` (vem da cache, quase instantâneo) |
| Código novo ou alterado desde a última vez | Só o que mudou é gerado de novo (as classes que contêm métodos alterados também atualizam o resumo) |
| O ficheiro deixaria de compilar/ficaria com erros de sintaxe | A escrita é **cancelada** e o ficheiro fica intacto |
| Ficheiro que não está em UTF-8 | É ignorado (para não corromper acentos) |
| Ficheiro com quebras de linha Windows (CRLF) | Mantêm-se |

## O dia a dia

- **Editas código** num ficheiro que tem blocos da IA e fazes commit: o commit leva só as tuas linhas.
- **`git status` / `git diff`** não mostram os blocos da IA como alterações.
- **Um `git pull` que altere o ficheiro, ou um `git stash`**, repõe a versão do repositório e os blocos
  desaparecem do disco. Corre `docsmaker doc --mode inline` outra vez: demora pouco, porque vem da cache.
- **Um `git checkout` normal** de um ficheiro que o git considera igual não apaga os blocos.

## Verificar por ti

Depois de gerar:

```
git status                      # não deve listar alterações causadas pela IA
git diff                        # idem
git show HEAD:caminho/Classe.java   # o que está no repositório: sem blocos de IA
```

E procura marcas no que está commitado (não deve devolver nada):

```
git grep docsmaker HEAD -- "*.java"
```

## Reverter o `init`

Num clone onde já não queres o filtro:

```
git config --local --remove-section filter.docsmaker-ai-doc
```

e apaga a linha `*.java filter=docsmaker-ai-doc` de `.git/info/attributes` (ou
do `.gitattributes`, se usaste `--shared`). Os blocos de IA que estiverem nos
ficheiros passam a poder entrar em commits, por isso remove-os antes
(apagando as linhas entre as marcas, ou repondo os ficheiros com o git).

## Limites a conhecer

- Os blocos só existem no teu disco; não há "sincronização" entre colegas.
- Um comentário `/** */` **vazio** escrito por uma pessoa também é removido do commit (o filtro apaga
  comentários de documentação vazios). É raro e inofensivo, mas é bom saber.
- Marcas partidas (um `ai-begin` sem `ai-end`) fazem o filtro **não alterar** o ficheiro e
  avisar no ecrã, em vez de arriscar estragar o commit.
- A combinação com `core.autocrlf=true` (conversão automática de fins de linha) não foi testada.
