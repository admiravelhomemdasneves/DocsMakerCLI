# Submissão: extração de regras de negócio com IA (`docsmaker rules`)

Extensão do docsmaker para a opção *"Project AI usage — documentation with AI /
extraction of embedded business rules from code"*.

## O problema

A análise estática do código (classes, métodos, assinaturas) gera boa
documentação **técnica**, mas não explica **porquê** uma regra existe. Em código
antigo, há limites, escalões e validações implementados há anos que ninguém sabe
justificar.

## O que foi feito

Um comando novo, `docsmaker rules`, que lê os métodos com lógica de negócio e
pede a um modelo de IA **local** (Ollama; o código nunca sai da máquina) que
explique, em linguagem natural, a regra que cada um implementa. Para cada regra
devolve:

- a **regra** (com os valores exatos que o código usa) e as suas **condições**;
- o **porquê provável**, sempre marcado como hipótese inferida;
- a **evidência**: linhas copiadas do código, **verificadas** contra o código real;
- uma **confiança** (`alta`/`média`/`baixa`), limitada a `baixa` se nenhuma
  linha de evidência for verificável;
- as **questões em aberto** que só alguém do negócio pode confirmar.

O resultado sai em consola, em JSON ou em ficheiros (`.docsmaker/rules/`), e
**nunca altera o código-fonte**.

## O que reutiliza e o que é novo

| Reutilizado da ferramenta original | Novo |
|------------------------------------|------|
| Análise com tree-sitter e o `CodeModel` | Sinais estruturais por método (decisões, `throw`, comparações, valores "mágicos") e seleção de candidatos por pontuação |
| Cliente Ollama local e as garantias "só local" | Prompt de extração, saída JSON forçada, leitor tolerante de respostas de modelos pequenos, uma repetição guiada |
| Portões de segredos (antes e depois do modelo), allowlist, auditoria | Verificação de evidência contra o código e limitação da confiança |
| Cache por hash do pedido | Regra de não guardar em cache respostas inutilizáveis |
| Camada `app`, CLI Cobra, escrita sidecar | Comando `rules`, relatório Markdown (pt/en), fixture com regras plantadas, avaliador contra gabarito |

Esforço adicional: ~4 300 linhas, das quais ~63 % são testes, documentação,
fixtures e scripts (~1 570 de código), numa branch própria com 7 commits, um por
passo do plano.

## Resultados

- **Testes:** `go vet` e `go test ./...` verdes (12 pacotes). Testes novos com um
  modelo falso para o encanamento, as defesas e os gates; as guardas da cache e
  dos gates foram verificadas por mutação (estragá-las faz um teste falhar).
- **Demo de ponta a ponta** (`scripts/demo.sh`, Ollama real): 46 verificações,
  15 delas novas (só os 5 métodos com decisões são analisados e nenhum getter,
  fontes intactas, segunda execução vinda da cache, sidecar, sem modo `inline`,
  credencial bloqueada ou redigida).
- **Qualidade medida com um modelo real** (`qwen2.5-coder:1.5b`, 3 repetições,
  fixture com 5 regras plantadas e 2 métodos técnicos como controlo):

  | | Resultado |
  |---|---|
  | Regras plantadas encontradas | 15/15 |
  | …com todos os valores certos | 9/15 |
  | Métodos técnicos tomados por regra (falsos positivos) | 5/6 |
  | Regras incompletas dadas como confiança alta | 4 de 6 |
  | Linhas de evidência inventadas e removidas | 52 |

## Limitações (a dizer com franqueza)

- Com um modelo pequeno, o "porquê" é quase sempre genérico ou inventado, e há
  muitos falsos positivos em código técnico. Por isso é apresentado como
  hipótese e acompanha-se de evidência e de questões em aberto. Não é uma
  especificação do sistema.
- A avaliação usa um só modelo, 5 regras e uma métrica por palavras-chave
  (aproximada). O modelo por omissão (`qwen2.5-coder:7b`) não foi medido.
- Cada método é analisado isoladamente: regras espalhadas por vários métodos,
  constantes de outras classes ou dados na base de dados não são vistos.
- Só Java. Um intermitente (1 em 3 execuções completas do demo, não reproduzido
  depois) falhou o passo do sidecar; a causa mais provável é o gate de segredos
  a bloquear texto do modelo em modo `--yes`, mas não foi confirmada.
- A primeira execução real do CI (Linux/macOS) continua pendente, como na
  primeira entrega.

## Para ler mais

- Utilizador: [capítulo 8](utilizador/08-regras-de-negocio.md)
- Desenho, defesas, testes e avaliação: [capítulo técnico 10](tecnico/10-extracao-de-regras.md)

## Mensagem às avaliadoras (rascunho)

> Olá, antes de avançar queria confirmar uma coisa. Proponho uma extensão ao
> docsmaker para a 2.ª prioridade: um módulo que usa IA local para explicar, em
> linguagem natural, as regras de negócio implementadas em código legado (o
> "porquê"), complementando a documentação técnica. Reutiliza a base já feita
> (análise, CLI, arquitetura), mas é entregue como funcionalidade autónoma:
> comando próprio, testes, documentação, demo e uma avaliação de qualidade
> medida. Contam esta extensão como 2.ª prioridade, ficando a ferramenta
> original como 1.ª, ou preferem duas entregas totalmente distintas? Ajusto o
> planeamento em conformidade.
