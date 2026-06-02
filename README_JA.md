# <img src="web/public/ongrid-logo.svg" alt="" width="40" align="absmiddle" style="vertical-align: middle;" /> ongrid

> **運用のための AI エージェント。** 各ホストに軽量エージェントを配置すると、Ongrid がメトリクス・ログ・トレース・トポロジー・ソースコードを横断的に分析し、自然言語で根本原因を特定します。
>
> *SRE・DevOps・プラットフォームチームのために。*

[![Go Report Card](https://goreportcard.com/badge/github.com/ongridio/ongrid)](https://goreportcard.com/report/github.com/ongridio/ongrid)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Tech](https://img.shields.io/badge/Tech-Go%20%7C%20TypeScript%20%7C%20React-blue)](#)

[English](./README.md) | [简体中文](./README_ZH.md) | 日本語 | [한국어](./README_KO.md) | [Español](./README_ES.md) | [Français](./README_FR.md) | [Deutsch](./README_DE.md) | [Português](./README_PT.md) | [Русский](./README_RU.md)

[インストール](#インストール) • [連携](#既存スタックとそのまま連携) • [ライセンス](#ライセンス)

---

<p align="center">
  <video src="docs/assets/demo.mp4" autoplay loop muted playsinline width="100%"></video>
</p>

## インストール

最新リリースの tarball をダウンロードし、インストーラーを実行します（Ubuntu 22.04+、Debian 12+、RHEL/Rocky 9）：

```bash
gh release download v0.7.167 --repo ongridio/ongrid -p 'ongrid-v0.7.167-linux-amd64.tar.xz*'
tar xf ongrid-v0.7.167-linux-amd64.tar.xz && cd ongrid-v0.7.167-linux-amd64
sudo ./install.sh
```

### またはソースから実行

ローカル開発: 管理者アカウントとモデル API キーを設定し、フルスタックを起動します。

```bash
cp deploy/.env.example deploy/.env
make compose-up    # make compose-down to stop
```

## 既存スタックとそのまま連携

チームの可観測性・チャネル・モデルスタックにそのまま組み込めます。

<table>
<tr>
  <td><b>可観測性</b></td>
  <td><img src="https://api.iconify.design/logos:prometheus.svg" alt="Prometheus" title="Prometheus" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:grafana.svg" alt="Grafana" title="Grafana" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/loki.svg" alt="Loki" title="Loki" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/tempo.svg" alt="Tempo" title="Tempo" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/opentelemetry.svg" alt="OpenTelemetry" title="OpenTelemetry" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:qdrant-icon.svg" alt="Qdrant" title="Qdrant" height="28" /></td>
</tr>
<tr>
  <td><b>チャネル</b></td>
  <td><img src="https://api.iconify.design/logos:slack-icon.svg" alt="Slack" title="Slack" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:telegram.svg" alt="Telegram" title="Telegram" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/larksuite.svg" alt="Larksuite" title="Larksuite" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/dingtalk.svg" alt="DingTalk" title="DingTalk" height="28" />&nbsp;&nbsp;<img src="https://cdn.simpleicons.org/wechat" alt="WeCom" title="WeCom" height="28" />&nbsp;&nbsp;<img src="https://api.iconify.design/logos:webhooks.svg" alt="Webhook" title="Webhook" height="28" /></td>
</tr>
<tr>
  <td><b>モデル</b></td>
  <td><img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/claude-color.svg" alt="Anthropic" title="Anthropic" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/openai.svg" alt="OpenAI" title="OpenAI" height="28" />&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/gemini-color.svg" alt="Gemini" title="Gemini" height="28" />&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/deepseek-color.svg" alt="DeepSeek" title="DeepSeek" height="28" />&nbsp;&nbsp;<img src="docs/assets/integrations/zhipu.svg" alt="Zhipu" title="Zhipu" height="28" />&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi-color.svg" alt="Kimi" title="Kimi" height="28" /></td>
</tr>
</table>

## ライセンス

Apache 2.0 — [LICENSE](LICENSE) を参照。
