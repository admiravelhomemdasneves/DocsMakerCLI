# 4. Referência de comandos

Convenções: `[path]` é opcional e por omissão é `.` (a pasta atual).
`docsmaker <comando> --help` mostra sempre a ajuda do próprio programa.
Qualquer erro termina com código de saída `1` e a mensagem vai para o stderr.

| Comando | Função |
|---------|--------|
| [`analyze`](#analyze) | Estrutura do projeto, sem IA |
| [`doc`](#doc) | Gerar documentação |
| [`init`](#init) | Preparar o git para a documentação dentro do código |
| [`model`](#model) | Gerir o Ollama e os modelos |
| [`allowlist`](#allowlist) | Gerir valores declarados "não é segredo" |
| `--version` | Versão instalada |

---

## analyze

```
docsmaker analyze [path] [--format text|json] [--out ficheiro]
```

Analisa o projeto e mostra pacotes, classes, campos, métodos, chamadas e
relações. Não usa o modelo de IA nem a rede.

| Opção | Por omissão | Descrição |
|-------|-------------|-----------|
| `--format` | `text` | `text` (árvore legível) ou `json` (para outras ferramentas) |
| `--out` | (consola) | Escreve num ficheiro em vez de mostrar na consola |

Ignora automaticamente `.git`, `target`, `build`, `out`, `node_modules`,
`.docsmaker` e o que o `.gitignore` do projeto exclui.

---

## doc

```
docsmaker doc [path] [opções]
```

Gera documentação com o modelo local.

| Opção | Por omissão | Descrição |
|-------|-------------|-----------|
| `--level` | `project` | `method`, `class`, `package` ou `project` |
| `--target` | | O que documentar. Obrigatório nos níveis `method`, `class` e `package` (ver abaixo) |
| `--mode` | `console` | `console`, `sidecar` ou `inline` (ver abaixo) |
| `--secrets` | `prompt` | Política de informação sensível: `prompt`, `strict` ou `off` |
| `--yes` | desligado | Nunca perguntar. Qualquer informação sensível **interrompe** a execução |
| `--model` | `qwen2.5-coder:7b` | Modelo do Ollama a usar |
| `--format` | `text` | `text` ou `json` |

### Formato do `--target`

| Nível | Formato | Exemplo |
|-------|---------|---------|
| `method` | `pacote.Classe#método` | `com.empresa.faturacao.Fatura#total` |
| `class` | `pacote.Classe` | `com.empresa.faturacao.Fatura` |
| `package` | `pacote` | `com.empresa.faturacao` |
| `project` | (não se usa) | |

> Métodos com o mesmo nome (sobrecarga) partilham o mesmo `--target`; é usado o primeiro.

### O que cada nível produz

Os níveis são cumulativos de baixo para cima: pedir uma classe também mostra os
seus métodos; pedir um pacote mostra as classes e métodos; pedir o projeto
mostra tudo e termina com o texto do README.

### Modos de saída (`--mode`)

| Modo | O que faz |
|------|-----------|
| `console` | Mostra o resultado no terminal. Não escreve nada nos teus ficheiros. |
| `sidecar` | Mostra **e** guarda em `.docsmaker/doc/<nível>/<alvo>.md`. Não toca no código. |
| `inline` | Escreve comentários `/** */` nos ficheiros `.java`. **Exige `docsmaker init`** no repositório (ver [capítulo 6](06-documentacao-no-codigo.md)). Só documenta métodos e classes; os níveis pacote e projeto continuam a aparecer na consola. |

### Política de informação sensível (`--secrets`)

| Valor | Comportamento |
|-------|---------------|
| `prompt` | Pergunta-te o que fazer a cada achado (ver [capítulo 5](05-informacao-sensivel.md)) |
| `strict` | Nunca pergunta: qualquer achado interrompe a execução |
| `off` | **Desliga a verificação** e mostra um aviso. Só para código sintético ou de teste |

`--yes` tem o mesmo efeito prático que `strict` (interrompe em vez de perguntar).
Valores já na [allowlist](#allowlist) passam em ambos.

### Saída `--format json`

Devolve um objeto com `artifacts` (lista de `level`, `target`, `content`) e, em
`--mode inline`, também `inline` (o que foi inserido, substituído, deixado igual
ou ignorado, ficheiro a ficheiro).

### Resumo do modo `inline`

No fim mostra uma linha como:

```
=== inline: 15 inserted, 0 replaced, 0 unchanged, 3 skipped in 6 file(s) ===
```

| Termo | Significado |
|-------|-------------|
| `inserted` | Comentário novo escrito |
| `replaced` | Um comentário de IA anterior foi atualizado |
| `unchanged` | Já estava igual |
| `skipped` | Deixado em paz (por exemplo, já tem um comentário escrito por uma pessoa); o motivo é listado |

---

## init

```
docsmaker init [path] [--force] [--shared]
```

Prepara **este clone** do repositório git para que os comentários escritos pela
IA nunca sejam incluídos num commit. Podes correr várias vezes: se já estiver
feito, não altera nada. Detalhes no [capítulo 6](06-documentacao-no-codigo.md).

| Opção | Descrição |
|-------|-----------|
| `--force` | Substitui um filtro diferente já configurado para os mesmos ficheiros |
| `--shared` | Regista a regra no `.gitattributes` (ficheiro versionado, partilhado com a equipa) em vez do ficheiro local `.git/info/attributes` |

Falha se `path` não estiver dentro de um repositório git.

---

## model

Gere o Ollama e os modelos. Só `model pull` pode usar a internet, e pede
confirmação antes de instalar ou descarregar.

### model status

```
docsmaker model status [--format text|json]
```

Diz se o Ollama está instalado e a correr (e a versão), em que endereço, e se o
modelo por omissão está descarregado. Se faltar algo, sugere o comando.

### model list

```
docsmaker model list [--format text|json]
```

Lista os modelos disponíveis localmente, com o tamanho. Arranca o Ollama se
estiver parado.

### model pull

```
docsmaker model pull [modelo] [--yes]
```

Instala o Ollama (se faltar), arranca-o (se parado) e descarrega o modelo
(por omissão `qwen2.5-coder:7b`). Pergunta `Proceed? [y/N]` antes de **cada**
instalação ou download; só `y`/`yes` aceita.

| Opção | Descrição |
|-------|-----------|
| `--yes` | Responde "sim" a estas confirmações (útil em scripts) |

Recusa sempre modelos "cloud" (por exemplo `…:120b-cloud`), porque correm em
servidores remotos.

### Variável de ambiente `OLLAMA_HOST`

Se o teu Ollama não usa o endereço habitual (`127.0.0.1:11434`), podes indicá-lo
com `OLLAMA_HOST`. **Só são aceites endereços desta máquina** (`localhost`,
`127.x.x.x`, `::1`); qualquer outro é recusado para garantir que o código não
sai do computador.

---

## allowlist

Gere os valores que declaraste "não é um segredo" (falsos alarmes).

```
docsmaker allowlist list                 [--project pasta]
docsmaker allowlist remove <id|padrão>   [--project pasta]
docsmaker allowlist add-pattern <regex>  [--project pasta]
```

| Subcomando | Função |
|------------|--------|
| `list` | Mostra as entradas (identificadas por um hash, nunca pelo valor) e os padrões |
| `remove` | Remove uma entrada (indica os primeiros 6+ caracteres do hash que o `list` mostra) ou um padrão (o texto exato) |
| `add-pattern` | Permite todos os valores que **correspondam por inteiro** à expressão regular |

Os padrões demasiado abrangentes (por exemplo `.*` ou `[a-z0-9]+`) são recusados,
porque desligariam a deteção. O ficheiro vive em `<projeto>/.docsmaker/allowlist.json`.

Normalmente não precisas de `add-pattern`: ao responder `w` a uma pergunta, o
valor é adicionado sozinho.
