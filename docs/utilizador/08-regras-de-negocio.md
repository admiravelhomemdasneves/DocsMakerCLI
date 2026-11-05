# 8. Regras de negócio

A documentação normal (`docsmaker doc`) diz **o que** o código faz. Em código
antigo, o que falta quase sempre é o **porquê**: aquele `if (total < 20)`, aquele
limite de 500, aquele cálculo de IVA por região. A regra foi implementada há
anos e já ninguém se lembra da razão.

O comando `docsmaker rules` tenta recuperar essas regras. Lê os métodos que
parecem conter lógica de negócio e pede ao modelo local que explique, em
linguagem natural, a regra que cada um implementa.

## O que tens de saber antes de confiar no resultado

O modelo só vê o código. Por isso:

- **A regra em si** (limiares, taxas, condições) lê-se diretamente do código e,
  em geral, é fiável. Cada regra traz as **linhas de código** em que assenta.
- **O "porquê"** é uma **hipótese**. É sempre apresentado como "Porquê provável
  (inferido)". Só quem conhece o negócio o pode confirmar, e é por isso que cada
  regra traz também as **questões em aberto**.
- A **confiança** (`alta`, `média`, `baixa`) diz quanto o modelo se apoiou no
  que está escrito. Uma regra sem nenhuma linha de evidência **verificável** no
  código nunca passa de `baixa`, diga o modelo o que disser.

> Trata o resultado como um ponto de partida para perguntares às pessoas certas,
> não como a especificação do sistema.

## Primeira utilização

```
docsmaker rules
```

Analisa o projeto da pasta atual e mostra o relatório na consola. Demora, porque
há uma chamada ao modelo por método; o progresso aparece no stderr
(`[3/12] com.empresa.faturacao.Desconto#calcular`). O mesmo código volta a sair
da cache nas execuções seguintes.

Opções que costumas querer:

```
docsmaker rules --target com.empresa.faturacao          # só um pacote (ou classe, ou método)
docsmaker rules --mode sidecar                          # guarda também em .docsmaker/rules/
docsmaker rules --min-confidence medium                 # esconde as regras menos fiáveis
docsmaker rules --max-methods 20                        # só os 20 métodos mais promissores
docsmaker rules --output-lang en                        # regras e relatório em inglês
```

## Como se escolhem os métodos

Enviar todos os métodos ao modelo seria lento e encheria o relatório de ruído.
Antes, o docsmaker mede cada método (sem IA): quantas decisões (`if`, `switch`,
`? :`), quantos `throw`, quantas comparações e quantos valores "mágicos"
(números diferentes de 0 e 1, textos). Daí sai uma pontuação:

```
pontuação = 2 × decisões + 2 × throws + comparações + valores mágicos
```

Só os métodos com pontuação **igual ou superior a 3** (`--min-score`) são
enviados. Getters, setters e atribuições simples têm pontuação 0 e nunca vão ao
modelo. Se um método que te interessa fica de fora, baixa o `--min-score`; se o
relatório vem cheio de ruído, sobe-o ou usa `--max-methods`.

## Ler o relatório

Para cada classe com regras, o relatório mostra, por método:

```
## com.empresa.faturacao.PoliticaDesconto

### descontoPara (linha 4) — Confiança: alta

**Regra:** Encomendas abaixo de 20 não têm desconto; a partir de 100 unidades há 15%.

**Condições:**

- totalEncomenda < 20
- quantidade >= 100

**Porquê provável (inferido):** Provavelmente um incentivo a compras em volume.

**Evidência:**

    if (totalEncomenda < 20.0) {
    if (quantidade >= 100) {

**Questões em aberto:**

- O limite de 20 inclui IVA?
```

> Este exemplo mostra o **formato**; o texto é ilustrativo.

| Parte | O que é |
|-------|---------|
| **Regra** | O que o código impõe, com os valores exatos que usa |
| **Condições** | Cada condição ou limiar, um por linha |
| **Porquê provável (inferido)** | A hipótese do modelo. Pode ser errada |
| **Evidência** | Linhas copiadas do código. Se o modelo citou uma linha que **não existe** no método, é removida e o relatório avisa |
| **Questões em aberto** | O que só alguém do negócio pode confirmar |

Depois das classes, a secção **Métodos sem resultado** lista os que foram
omitidos (ver abaixo) ou em que o modelo não deu uma resposta utilizável, mesmo
após uma segunda tentativa. Os métodos em que o modelo não encontrou nenhuma
regra só entram na contagem do resumo.

## Onde fica o resultado

| `--mode` | O que faz |
|----------|-----------|
| `console` (por omissão) | Mostra o relatório. Não escreve nada |
| `sidecar` | Mostra **e** guarda `.docsmaker/rules/RULES.md` (tudo) e um ficheiro por classe com regras, por exemplo `com.empresa.faturacao.PoliticaDesconto.md` |

O `rules` **nunca altera o teu código-fonte** e não tem modo `inline`.
Um ficheiro de classe de uma execução anterior não é apagado se a classe deixar
de ter regras; apaga a pasta `.docsmaker/rules/` se quiseres recomeçar.

Para outras ferramentas, `--format json` devolve o mesmo resultado
estruturado (ver a [referência](04-referencia-de-comandos.md#rules)).

## Informação sensível

As mesmas verificações do `doc` aplicam-se: o código do método e as declarações
dos campos da classe passam pelo portão de segredos **antes** de chegarem ao
modelo, e a resposta passa por outro **antes** de aparecer. Se responderes `s`
(saltar) a um achado, o método aparece como "omitido (conteúdo sensível)" e o
modelo nem é chamado. Ver o [capítulo 5](05-informacao-sensivel.md).

## Limites

- **Só Java**, como o resto da ferramenta.
- **Uma regra por método.** Se um método contém várias regras relacionadas, o
  modelo resume-as todas na mesma entrada (as condições ficam listadas à parte).
- **Regras espalhadas por vários métodos** (um método chama outro que tem o
  limite) não são ligadas entre si: cada método é analisado isoladamente.
- **Sobrecargas** são analisadas e identificadas separadamente (pela linha), e
  `--target pacote.Classe#método` inclui todas as que têm esse nome (ao contrário
  do `doc`, que escolhe só a primeira).
- **A qualidade depende do modelo.** Medido com o modelo pequeno `qwen2.5-coder:1.5b`
  numa fixture com regras conhecidas: encontrou todas as regras, mas só cerca de
  60% (9 de 15) saíram com todos os valores certos; em métodos puramente técnicos
  (formatação de texto) dizia quase sempre que havia uma regra (e por vezes com confiança
  `alta`); e a confiança `alta` apareceu também em regras incompletas. O "porquê" foi quase sempre genérico.
  Para código importante, usa um modelo maior (`--model`) e revê sempre. Os
  detalhes estão no [capítulo técnico 10](../tecnico/10-extracao-de-regras.md#71-avaliação-com-um-modelo-real).
- Os valores de configuração que não estão no ficheiro do método (uma constante
  noutra classe, uma tabela na base de dados) não são vistos pelo modelo.

Passo seguinte: [Perguntas frequentes e problemas](07-perguntas-frequentes.md).
