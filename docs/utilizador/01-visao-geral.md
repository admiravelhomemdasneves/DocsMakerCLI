# 1. Visão geral

## O que é

O docsmaker lê o teu código Java e escreve a documentação dele:

- uma **descrição de cada método** (o que faz e porquê);
- uma **descrição de cada classe** (a responsabilidade e como colabora com as outras);
- um **resumo de cada pacote**;
- um **README do projeto** (visão geral escrita por IA, mais a estrutura real de pacotes e classes);
- em vários **níveis de detalhe**: método, classe, pacote ou projeto inteiro.

A escrita é feita por um modelo de IA, mas esse modelo corre **no teu computador**
(através do [Ollama](https://ollama.com)). Não há conta, chave de API nem serviço
na nuvem.

## O que garante

| Garantia | O que significa na prática |
|----------|----------------------------|
| **O código não sai da máquina** | A ferramenta só comunica com o Ollama local. Um Ollama noutra máquina e os modelos "cloud" são recusados. |
| **Nada treina modelos** | Como tudo corre localmente, o teu código nunca é enviado a ninguém, logo nunca serve para treino. |
| **Informação sensível é apanhada** | Passwords, chaves, tokens e ligações a bases de dados são detetados *antes* de chegarem ao modelo e *antes* de aparecerem em qualquer resultado. Decides tu o que fazer. |
| **Tu escolhes onde fica o resultado** | Só na consola, em ficheiros à parte, ou dentro dos próprios ficheiros de código. |
| **A IA não chega ao repositório central** | Se escreveres a documentação dentro do código, o git remove essas linhas em cada commit, mesmo num ficheiro que levas outras alterações tuas. |
| **Um único executável** | Não precisas de instalar Java, Python, Node, etc. para usar o docsmaker. |

> Estas garantias são verificadas automaticamente por um teste de ponta a ponta
> (`scripts/demo.sh`). Ver [Perguntas frequentes](07-perguntas-frequentes.md#como-verifico-as-garantias).

## O que não faz

- **Só analisa Java** (por agora).
- **O texto da IA é um rascunho.** Um modelo pequeno pode errar ou ser vago. Revê antes de confiar.
- **Não altera a lógica do teu código.** Só acrescenta comentários (e apenas quando pedes `--mode inline`).
- **Não substitui o teu `README.md`.** Produz o texto do README para a consola ou para um ficheiro à parte; copiá-lo para o projeto é um passo teu.
- **Não é uma sandbox de rede.** Garante que *ele próprio* só fala com o Ollama local; não controla o que outros programas fazem.
- **A deteção de segredos não é perfeita.** Usa padrões e estatística; pode haver falsos alarmes e, mais raramente, segredos que passam. Por isso a decisão final é tua.

## Como se encaixa no teu trabalho

```
 o teu projeto Java ──► docsmaker ──► consola / ficheiros à parte / comentários no código
                          │
                          ├─ analisa a estrutura (sem IA)
                          ├─ verifica segredos
                          └─ pede ao Ollama local que escreva as descrições
```

Passo seguinte: [Instalação](02-instalacao.md).
