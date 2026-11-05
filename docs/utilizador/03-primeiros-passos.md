# 3. Primeiros passos

Assume que já fizeste a [instalação](02-instalacao.md). Os exemplos usam um
projeto Java em `C:\projetos\meu-projeto` (em Linux/macOS, usa o teu caminho).

Em todos os comandos, o caminho do projeto é opcional e por omissão é a pasta
atual: `docsmaker doc` equivale a `docsmaker doc .`.

## 1. Ver a estrutura (sem IA)

```
docsmaker analyze C:\projetos\meu-projeto
```

Mostra pacotes, classes, campos, métodos e as relações entre classes. É rápido,
não usa o modelo e serve para confirmar que o docsmaker "entende" o projeto.
Se uma classe não aparece aqui, não vai aparecer na documentação.

## 2. Documentar uma classe

```
docsmaker doc C:\projetos\meu-projeto --level class --target com.empresa.faturacao.Fatura
```

`--target` é o nome completo da classe (pacote + nome). O resultado aparece na
consola: primeiro uma descrição de cada método, depois a da classe.

Para um só método:

```
docsmaker doc C:\projetos\meu-projeto --level method --target com.empresa.faturacao.Fatura#total
```

## 3. Documentar o projeto inteiro

```
docsmaker doc C:\projetos\meu-projeto
```

Percorre todos os pacotes e termina com o texto de um **README do projeto**.
Em projetos grandes demora (o modelo corre no teu computador); a segunda vez é
muito mais rápida, porque o que não mudou vem da cache.

## 4. Escolher onde fica o resultado

| Quero... | Usa |
|----------|-----|
| Ver na consola (por omissão) | `--mode console` |
| Guardar em ficheiros à parte, sem tocar no código | `--mode sidecar` → fica em `.docsmaker/doc/` |
| Escrever comentários nos ficheiros `.java` | `--mode inline` (precisa de um passo prévio: [documentação dentro do código](06-documentacao-no-codigo.md)) |

Exemplo:

```
docsmaker doc C:\projetos\meu-projeto --mode sidecar
```

Cria, por exemplo, `.docsmaker/doc/class/com.empresa.faturacao.Fatura.md` e
`.docsmaker/doc/project/<nome-do-projeto>.md` (este último é o texto do README).

## 5. Se aparecer uma pergunta sobre informação sensível

Se o docsmaker encontrar algo que parece uma password, chave ou token, **pára e
pergunta-te** antes de continuar:

```
[!] Potential sensitive information detected (connection-string)
    where: prompt for class com.empresa.config.Base (...Base.java:8)
    value: jdb***db (50 chars)
    [r] redact and continue  [s] skip this snippet  [a] allow once (include as-is)
    [w] not a secret: include it and never ask again for this value  [q] abort run
    choice [r]:
```

Em caso de dúvida carrega Enter: a opção por omissão (`r`) esconde o valor.
Todas as opções estão explicadas em [Informação sensível](05-informacao-sensivel.md).

## O que foi criado no teu projeto

A ferramenta cria uma pasta `.docsmaker/` dentro do projeto:

| Ficheiro / pasta | Conteúdo |
|------------------|----------|
| `.docsmaker/cache/` | Respostas já geradas (para não repetir trabalho) |
| `.docsmaker/audit.log` | Registo das tuas decisões sobre informação sensível (valores sempre mascarados) |
| `.docsmaker/allowlist.json` | Valores que declaraste "não é segredo" (só guarda um hash) |
| `.docsmaker/doc/` | Resultado do `--mode sidecar` |

Esta pasta é **local**: não deve ir para o repositório. O comando `docsmaker init`
(ver [capítulo 6](06-documentacao-no-codigo.md)) exclui-a do git automaticamente;
num projeto onde não o uses, acrescenta `.docsmaker/` ao teu `.gitignore`.

Passo seguinte: [Referência de comandos](04-referencia-de-comandos.md).
