---
title: トラブルシューティング
description: 本番で遭遇しやすい障害パターンの症状→原因→対処マップ。
---

## run が `Error` で終わる

`Error` は run のインフラ自体が壊れたことを意味します — verdict は
存在しません(チェックが実行された上で何かに到達できなかった `Failed`
とは異なります)。まず run の condition を見てください:

```bash
kubectl get checkrun <name> -o jsonpath='{.status.conditions}' | jq
```

| condition / 原因 | 意味 | 対処 |
|---|---|---|
| `RunnerServiceAccountMissing` | スイートの namespace に runner の ServiceAccount が無い | chart の `checkNamespaces` に namespace を追加して `helm upgrade` |
| `DeadlineExceeded` | run が `spec.timeout`(デフォルト 10m)を超過 | `timeout` を上げる、または runner pod が遅い/スケジュールできない理由を調査(`kubectl describe job`) |
| runner Job の失敗 | pod がスケジュールできない、または crash | `kubectl describe job -n <ns>` — 典型的には `nodeSelector` に合うノードが無い、taint に対する toleration が無い |

`Error` の run は**チェック結果を生まない**ため、状態 gauge
(`verikube_check_last_result`)は前回の値を保持し続けます。
`verikube_checkruns_total{phase="Error"}` と staleness でアラートして
ください — [Observability](/verikube/ja/guides/observability/) 参照。

## チェックが `unknown check type` で fail する

runner イメージがインストール済みの CRD より古く、知らない check type を
受け取って(黙ってスキップせず)明示的な失敗として報告しています。chart で
`runnerImage` を上書きしていると起きます。上書きを外す(空 = operator の
イメージを使用)か、バージョンを揃えてください。

## スイートがスケジュールで実行されない

- スケジュールは **UTC** で評価されます — `0 9 * * *` はローカル時間では
  なく 09:00 UTC に発火します。
- `spec.suspend` を確認してください — suspend 中のスイートはスケジュール
  実行をスキップします(手動トリガーは動きます)。
- `startingDeadline`(デフォルト 200s)より古い tick は設計どおりスキップ
  されます。キャッチアップせず、*次の* tick で再び発火します。

## run-now アノテーションが効かない

トリガーはアノテーションの**値が変わったとき**に発火します。同じ値の
再適用は no-op です(前回処理された値は `status.lastManualTrigger` に
記録されています)。タイムスタンプを使ってください:

```bash
kubectl annotate checksuite <name> \
  verikube.dev/run-now="$(date +%s)" --overwrite
```

## ローカルアドレスへのチェックが拒否される

loopback / link-local アドレス(cloud metadata の `169.254.169.254`
など)を対象とするチェックは、SSRF 対策としてデフォルトで拒否されます。
意図的にそうしたターゲットを probe する場合は、
[セキュリティモデル](/verikube/ja/security/)を読んだ上で chart の values
で `allowLocalTargets: true` を設定してください。

## メトリクスのクエリに `exported_namespace` が出てくる

scrape config に `honor_labels: true` がありません: メトリクス自身の
`namespace` ラベル(CheckSuite の namespace)がターゲットの namespace
ラベルと衝突し、Prometheus が `exported_namespace` にリネームしました。
chart の ServiceMonitor は自動で設定します。自前の scrape config でも
同じ設定が必要です。[Observability](/verikube/ja/guides/observability/)
を参照してください。

## `ForeignResultEntry` warning イベント

controller が、自身の Job と pod 名が一致しない結果エントリを CheckRun
の status に発見しました — namespace 内の何かが status に書き込んで
います。check namespace 内では pod-create 権限があれば runner の
ServiceAccount をマウントできるため、その namespace で誰が pod を作れる
かを見直すシグナルとして扱ってください。背景は
[セキュリティモデル](/verikube/ja/security/)にあります。
