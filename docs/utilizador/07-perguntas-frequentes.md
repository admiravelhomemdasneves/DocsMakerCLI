# 7. Perguntas frequentes e resolução de problemas

## Perguntas gerais

### O meu código é enviado para algum lado?
Não. O docsmaker só fala com o Ollama que corre na tua máquina. Um `OLLAMA_HOST`
que aponte para outro computador é recusado, e os modelos "cloud" do Ollama
também. A única atividade de rede é o `docsmaker model pull` (que descarrega o
modelo, e pede confirmação antes).

### Posso usar sem internet?
Sim, depois de o modelo estar descarregado. Gerar documentação não precisa de rede.

### Porque é lento?
O modelo corre no teu computador. Modelos maiores escrevem melhor e demoram mais.
A primeira execução num projeto é a mais lenta; as seguintes reutilizam a cache.
Para experimentar rápido, usa um modelo mais pequeno com `--model`.

### A documentação está fraca ou imprecisa.
Depende do modelo. Experimenta um maior (`docsmaker model pull qwen2.5-coder:7b`
e `--model qwen2.5-coder:7b`). Trata sempre o texto da IA como rascunho.

### Posso documentar linguagens que não são Java?
Ainda não. A estrutura prevê outras linguagens, mas só o Java está implementado.

### Onde está o README gerado?
No fim de `docsmaker doc` (nível `project`), na consola; com `--mode sidecar` fica também em
`.docsmaker/doc/project/<nome>.md`. **Não substitui** o `README.md` do teu projeto: se o quiseres,
copia o texto.

### Como verifico as garantias?
Se tens o repositório da ferramenta: `scripts/demo.sh` cria um repositório git descartável, corre o
fluxo todo (incluindo uma credencial falsa) e verifica, com commits reais, que nada de sensível
nem de IA chega ao git. Termina com erro se alguma garantia falhar. Com `DEMO_PAUSE=1` pára entre passos.

---

## Mensagens de erro

### `local Ollama is not available` / `Ollama is not installed`
O Ollama não está instalado. Corre `docsmaker model pull` (pede confirmação antes de instalar).
No Linux, instala-o a partir de https://ollama.com/download e corre o comando outra vez.

### `model "…" is not available locally; run docsmaker model pull …`
O modelo pedido não está descarregado. Corre o comando indicado, ou usa `--model` com um modelo
que `docsmaker model list` mostre.

### `OLLAMA_HOST points at "…", which is not this machine`
A variável de ambiente `OLLAMA_HOST` aponta para outra máquina. Isto é recusado de propósito.
Remove-a ou aponta-a para `localhost`.

### `model "…" is a cloud model that runs on remote servers`
Modelos com sufixo `cloud` correm nos servidores do Ollama, o que quebraria a garantia de o código
não sair da máquina. Escolhe um modelo local.

### `sensitive information blocked (…) in prompt for … (ficheiro:linha)`
Estás em `--yes` ou `--secrets strict` e foi encontrado algo que parece sensível. A mensagem diz onde.
Opções: tirar a credencial do código; se for um falso alarme, corre uma vez **sem** `--yes` e responde
`w` (fica na allowlist); ou usa `docsmaker allowlist add-pattern`.

### `confirmation prompt could not be answered` / `stdin closed`
Foi preciso perguntar-te algo mas não há terminal (por exemplo, num script). Usa `--secrets strict` para
falhar de forma previsível, ou resolve antes os falsos alarmes com a allowlist.

### `refusing --mode inline: the docsmaker git filter is not configured…`
Falta o `docsmaker init` neste clone. Corre-o e tenta de novo. Ver [capítulo 6](06-documentacao-no-codigo.md).

### `N file(s) are not covered by the docsmaker git filter`
O filtro está configurado, mas há ficheiros `.java` que o git não passa por ele. O caso típico é um
repositório aninhado ou *submodule* dentro do projeto, que tem a sua própria configuração: corre
`docsmaker init` também dentro dele. A mensagem indica o primeiro ficheiro afetado.

### `already assigns another filter to *.java … re-run with --force`
O `init` encontrou outro filtro para os mesmos ficheiros e não o substitui sem confirmação. Usa `--force`
se tiveres a certeza.

### `… would not be valid after insertion, left untouched`
A verificação detetou que o ficheiro ficaria com erros ou perderia declarações, e cancelou a escrita.
O ficheiro está intacto. Se acontecer de forma repetida, guarda o ficheiro e reporta o caso.

### `allowlist … is not valid`
O ficheiro `.docsmaker/allowlist.json` foi editado à mão ou está corrompido. Corrige-o ou apaga-o
(perdes só as permissões guardadas; voltarás a ser perguntado).

### `git executable not found in PATH`
O `init` e o `--mode inline` precisam do git instalado e no `PATH`.

### `no java files found under …`
A pasta indicada não tem ficheiros `.java` (ou estão todos em pastas ignoradas como `target/` ou excluídos
pelo `.gitignore`). Confirma o caminho com `docsmaker analyze`.

### `type "…" not found in package "…"` / `method "…" not found`
O `--target` não corresponde a nada. Corre `docsmaker analyze` para ver os nomes exatos
(pacote completo, maiúsculas/minúsculas, `Classe#método`).

### `--target "…" matches no package, class or method`
O mesmo, no comando `rules`: aceita `pacote`, `pacote.Classe` ou `pacote.Classe#método`.

### `rules does not support --mode inline`
O `rules` só mostra o relatório ou guarda-o em `.docsmaker/rules/` (`--mode sidecar`). Não escreve nas fontes.

---

## Problemas frequentes

### "A primeira execução foi demorada e a segunda foi instantânea"
É o esperado: a cache em `.docsmaker/cache/` guarda os resultados. Se mudares o modelo, o código ou os
prompts, só o que depende disso é regenerado.

### "O `rules` diz 'no candidate methods' / não encontrou nada"
Nenhum método no âmbito atinge a pontuação mínima (`--min-score`, por omissão 3). Os métodos sem decisões
(getters, atribuições) não são analisados. Baixa o `--min-score` ou alarga o `--target`. Ver o
[capítulo 8](08-regras-de-negocio.md#como-se-escolhem-os-métodos).

### "O `rules` marca um método como 'falhou'"
O modelo não devolveu uma resposta no formato pedido, nem à segunda tentativa. É mais frequente com modelos
pequenos. Volta a correr (só esse método é pedido de novo) ou usa um modelo maior com `--model`.

### "Quero começar do zero"
Apaga a pasta `.docsmaker/` do projeto (cache, registos e allowlist).

### "O Windows diz que o ficheiro não é de confiança"
Os executáveis não estão assinados. Verifica o `.sha256` do arquivo e, se a tua organização o permitir,
escolhe "Executar mesmo assim".

### "Num Linux/macOS não funciona"
Os pacotes para estes sistemas são construídos pela integração contínua mas ainda não foram validados em
máquinas reais. Reporta o problema com a saída de `docsmaker --version` e do erro.
