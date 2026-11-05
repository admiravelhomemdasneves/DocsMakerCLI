# 2. Instalação

São dois passos: instalar o **docsmaker** e preparar o **modelo de IA** (uma só vez).

## Requisitos

- Windows 10/11, Linux ou macOS. (O Windows é a plataforma testada até agora; os pacotes de Linux e
  macOS são construídos pela integração contínua mas ainda não foram validados em máquinas reais.)
- Espaço em disco para o modelo de IA (alguns GB; depende do modelo).
- RAM suficiente para o modelo escolhido (modelos maiores escrevem melhor mas pedem mais memória).
- Para a documentação dentro do código (`--mode inline`): o **git** instalado.

Não precisas de Java, Go, Python nem Node para *usar* o docsmaker.

## Passo 1 — instalar o docsmaker

1. Descarrega o arquivo para o teu sistema na página de *releases* do repositório:
   - Windows: `docsmaker_<versão>_windows_amd64.zip`
   - Linux / macOS: `docsmaker_<versão>_<sistema>_<arquitetura>.tar.gz`
2. (Opcional, recomendado) Confirma que o ficheiro não foi alterado, comparando com o
   ficheiro `.sha256` que vem ao lado.
3. Extrai o arquivo e corre o instalador que vem lá dentro:

| Sistema | Comando (na pasta extraída) |
|---------|------------------------------|
| Windows (PowerShell) | `.\install.ps1` |
| Linux / macOS | `./install.sh` |

O instalador copia o programa para a tua pasta de utilizador
(`%LOCALAPPDATA%\Programs\docsmaker` no Windows, `~/.local/bin` nos outros),
**sem pedir permissões de administrador** e **sem descarregar nada**. No Windows
acrescenta essa pasta ao `PATH` do teu utilizador; no Linux/macOS diz-te o que
fazer se a pasta não estiver no `PATH`.

4. **Abre um terminal novo** e confirma:

```
docsmaker --version
```

> **Aviso do sistema:** os executáveis não estão assinados digitalmente. O
> Windows (SmartScreen) ou o macOS (Gatekeeper) podem avisar na primeira
> execução. Se a tua organização exigir binários assinados, fala com quem
> mantém a ferramenta.

## Passo 2 — preparar o modelo de IA

```
docsmaker model pull
```

Este comando prepara tudo o que falta, **e pergunta antes de cada passo que
instala ou descarrega alguma coisa**:

1. Se o Ollama **não está instalado**, propõe instalá-lo (no Windows com `winget`,
   no macOS com `brew`). No Linux não instala nada sozinho: indica-te a página
   de download oficial.
2. Se o Ollama está instalado mas **parado**, arranca-o.
3. Se o **modelo** não está descarregado, pergunta se o podes descarregar do
   registo do Ollama (pode ter vários GB).

Responde `y` para aceitar. Qualquer outra resposta (incluindo carregar só em
Enter) é um **não**, e nada é instalado nem descarregado.

Só este comando usa a internet. O que é descarregado é o modelo; **nada do teu
código é enviado**.

Para escolher outro modelo:

```
docsmaker model pull qwen2.5-coder:1.5b
```

Modelos mais pequenos são mais rápidos mas escrevem pior. O modelo por omissão
é `qwen2.5-coder:7b`.

## Verificar que está tudo pronto

```
docsmaker model status
```

Deve mostrar `installed: true`, `running: true` e o modelo por omissão
descarregado. Se algo faltar, a última linha diz o comando que resolve.

## Desinstalar

- Windows: apaga `%LOCALAPPDATA%\Programs\docsmaker` e remove essa pasta do `PATH` do utilizador.
- Linux/macOS: apaga `~/.local/bin/docsmaker`.
- Em cada projeto onde o usaste, podes apagar a pasta `.docsmaker/` (cache e registos).
- Se usaste `docsmaker init` num repositório, vê [como reverter](06-documentacao-no-codigo.md#reverter-o-init).

Passo seguinte: [Primeiros passos](03-primeiros-passos.md).
