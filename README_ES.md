# <img src="web/public/ongrid-logo.svg" alt="" width="40" align="absmiddle" style="vertical-align: middle;" /> ongrid

> **Un agente de IA para Operaciones.** Pon un agente ligero en cada host; Ongrid analiza tus métricas, logs, trazas, topología y código fuente para identificar la causa raíz en lenguaje natural.
>
> *Hecho para equipos de SRE, DevOps y plataforma.*

[![Go Report Card](https://goreportcard.com/badge/github.com/ongridio/ongrid)](https://goreportcard.com/report/github.com/ongridio/ongrid)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Tech](https://img.shields.io/badge/Tech-Go%20%7C%20TypeScript%20%7C%20React-blue)](#)

[English](./README.md) | [简体中文](./README_ZH.md) | [日本語](./README_JA.md) | [한국어](./README_KO.md) | Español | [Français](./README_FR.md) | [Deutsch](./README_DE.md) | [Português](./README_PT.md) | [Русский](./README_RU.md)

[Instalación](#instalación) • [Compatible](#compatible-con-tu-stack) • [Licencia](#licencia)

---

<p align="center">
  <video src="docs/assets/demo.mp4" autoplay loop muted playsinline width="100%"></video>
</p>

## Instalación

Descarga el tarball de la última release y ejecuta el instalador (Ubuntu 22.04+, Debian 12+, RHEL/Rocky 9):

```bash
gh release download v0.7.167 --repo ongridio/ongrid -p 'ongrid-v0.7.167-linux-amd64.tar.xz*'
tar xf ongrid-v0.7.167-linux-amd64.tar.xz && cd ongrid-v0.7.167-linux-amd64
sudo ./install.sh
```

### O ejecutar desde el código fuente

Desarrollo local: configura la cuenta de admin y una API key de modelo, y levanta todo el stack.

```bash
cp deploy/.env.example deploy/.env
make compose-up    # make compose-down to stop
```

## Compatible con tu stack

Se integra con los stacks de observabilidad, canales y modelos que tu equipo ya usa.

<table>
<tr>
  <td><b>Observabilidad</b></td>
  <td><img src="https://api.iconify.design/logos:prometheus.svg" alt="Prometheus" title="Prometheus" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:grafana.svg" alt="Grafana" title="Grafana" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/loki.svg" alt="Loki" title="Loki" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/tempo.svg" alt="Tempo" title="Tempo" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/opentelemetry.svg" alt="OpenTelemetry" title="OpenTelemetry" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:qdrant-icon.svg" alt="Qdrant" title="Qdrant" height="28" /></td>
</tr>
<tr>
  <td><b>Canales</b></td>
  <td><img src="https://api.iconify.design/logos:slack-icon.svg" alt="Slack" title="Slack" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:telegram.svg" alt="Telegram" title="Telegram" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/larksuite.svg" alt="Larksuite" title="Larksuite" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/dingtalk.svg" alt="DingTalk" title="DingTalk" height="28" />&nbsp;&nbsp;<img src="https://cdn.simpleicons.org/wechat" alt="WeCom" title="WeCom" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:webhooks.svg" alt="Webhook" title="Webhook" height="28" /></td>
</tr>
<tr>
  <td><b>Modelos</b></td>
  <td><img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/claude-color.svg" alt="Anthropic" title="Anthropic" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/openai.svg" alt="OpenAI" title="OpenAI" height="28" />&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/gemini-color.svg" alt="Gemini" title="Gemini" height="28" />&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/deepseek-color.svg" alt="DeepSeek" title="DeepSeek" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/zhipu.svg" alt="Zhipu" title="Zhipu" height="28" />&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi-color.svg" alt="Kimi" title="Kimi" height="28" /></td>
</tr>
</table>

## Licencia

Apache 2.0 — ver [LICENSE](LICENSE).
