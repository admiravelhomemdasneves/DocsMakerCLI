# Documentação do docsmaker

O **docsmaker** gera documentação de código (descrições de classes e métodos,
resumos de pacotes e um README de projeto) com um modelo de IA que corre
**na tua própria máquina**. Hoje analisa Java; a arquitetura permite outras
linguagens.

Esta pasta tem duas partes, para públicos diferentes.

## Para quem usa a ferramenta — [`utilizador/`](utilizador/)

Não é preciso saber Go nem como a ferramenta funciona por dentro.

| # | Documento | Para quê |
|---|-----------|----------|
| 1 | [Visão geral](utilizador/01-visao-geral.md) | O que faz, o que garante, o que não faz |
| 2 | [Instalação](utilizador/02-instalacao.md) | Instalar o programa e preparar o modelo de IA |
| 3 | [Primeiros passos](utilizador/03-primeiros-passos.md) | Gerar a primeira documentação em 5 minutos |
| 4 | [Referência de comandos](utilizador/04-referencia-de-comandos.md) | Todos os comandos e opções |
| 5 | [Informação sensível](utilizador/05-informacao-sensivel.md) | Passwords, chaves e tokens: o que acontece e como decidir |
| 6 | [Documentação dentro do código](utilizador/06-documentacao-no-codigo.md) | Escrever nos ficheiros sem que a IA chegue ao repositório |
| 7 | [Perguntas frequentes e problemas](utilizador/07-perguntas-frequentes.md) | Mensagens de erro e como resolver |

## Para quem desenvolve a ferramenta — [`tecnico/`](tecnico/)

| # | Documento | Para quê |
|---|-----------|----------|
| 1 | [Arquitetura](tecnico/01-arquitetura.md) | Camadas, pacotes e portos (hexagonal) |
| 2 | [Pipeline de documentação](tecnico/02-pipeline-de-documentacao.md) | Sumarização hierárquica, cache, prompts |
| 3 | [Análise de Java](tecnico/03-analise-java.md) | Tree-sitter, duas passagens, o que é e não é resolvido |
| 4 | [Segurança e segredos](tecnico/04-seguranca-e-segredos.md) | Garantias, como são impostas, riscos residuais |
| 5 | [Inserção e filtro git](tecnico/05-insercao-e-filtro-git.md) | Como a IA fica fora dos commits |
| 6 | [Ollama e modelos](tecnico/06-ollama-e-modelos.md) | Cliente, bootstrap, restrições de rede |
| 7 | [Extensibilidade](tecnico/07-extensibilidade.md) | Novas linguagens, detetores, uma interface gráfica |
| 8 | [Build, testes e CI](tecnico/08-build-testes-e-ci.md) | Compilar, testar, empacotar, publicar |
| 9 | [Decisões e limitações](tecnico/09-decisoes-e-limitacoes.md) | Porquê assim, e o que fica por fazer |

O [README da raiz](../README.md) é um resumo rápido em inglês.
