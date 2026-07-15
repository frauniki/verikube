---
title: Observability
description: チェック結果を Prometheus メトリクスとして公開し、Grafana で可視化する。
---

VeriKube のデータフローは scrape を単純に保ちます: runner pod は結果を CheckRun の `.status` に報告し(server-side apply)、operator が完了した run を自身の `/metrics` エンドポイントのメトリクスに変換します。runner pod は短命な Job pod であり、自身ではメトリクスを公開しません。

```text
runner pods ──SSA──▶ CheckRun .status ──▶ operator /metrics ──scrape──▶ Prometheus ──▶ Grafana
```

## メトリクス

| メトリクス | 型 | ラベル | 意味 |
|---|---|---|---|
| `verikube_check_last_result` | gauge | `namespace,suite,check` | 最後に完了した run の verdict: 1 = 全 pod で pass、0 = 少なくとも 1 つで fail |
| `verikube_check_duration_seconds` | histogram | `namespace,suite,check,result` | probe 単位のレイテンシ(1ms–32s のバケット) |
| `verikube_check_result_total` | counter | `namespace,suite,check,result` | 累積 verdict(`result` = `pass`/`fail`) |
| `verikube_checkruns_total` | counter | `namespace,suite,phase` | 終了した run(`Succeeded`/`Failed`/`Error`) |
| `verikube_checkrun_duration_seconds` | histogram | `namespace,suite` | run 全体の所要時間 |
| `verikube_checkrun_last_completion_timestamp_seconds` | gauge | `namespace,suite` | スイートが最後に**結果付きの** run を完了した時刻 |

知っておくべきセマンティクス:

- `namespace`/`suite` は CheckSuite を識別します。ラベルには意図的に runner 名と pod 名を含めず、カーディナリティを自分が定義した範囲に抑えます。
- phase `Error` で終わった run(runner Job の失敗、deadline 超過、ServiceAccount 不在)は**チェック結果を生みません**。そのため 2 つの gauge は前回の値を保持します — staleness gauge と `verikube_checkruns_total{phase="Error"}` を組み合わせて検知してください。
- CheckSuite を削除すると、その gauge シリーズは削除されます。

## kube-prometheus-stack でのセットアップ

1. [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)をインストールします(prometheus-operator 系ならどれでも動きます):

   ```bash
   helm install monitoring prometheus-community/kube-prometheus-stack \
     --namespace monitoring --create-namespace
   ```

2. verikube のメトリクスを有効化します。メトリクスエンドポイントは認証・認可付きの HTTPS で提供されるため、scrape する Prometheus には nonResourceURL `/metrics` への `get` が必要です — `metrics.reader.*` が chart 付属の ClusterRole をあなたの Prometheus の ServiceAccount にバインドします:

   ```bash
   helm upgrade verikube oci://ghcr.io/frauniki/charts/verikube --reuse-values \
     --set metrics.enabled=true \
     --set metrics.serviceMonitor.enabled=true \
     --set metrics.reader.serviceAccountName=monitoring-kube-prometheus-prometheus \
     --set metrics.reader.namespace=monitoring
   ```

   (ServiceAccount 名は `kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceAccountName}'` で確認できます。)

3. run をトリガーしてターゲットを確認します:

   ```bash
   kubectl annotate checksuite <name> verikube.dev/run-now="$(date +%s)" --overwrite
   ```

   Prometheus UI で verikube ターゲットが **UP** になり、run 完了後に `verikube_check_last_result` がシリーズを返すはずです。

chart の ServiceMonitor は `honorLabels: true` を設定しており、メトリクス自身の `namespace` ラベル(CheckSuite の namespace)が scrape を生き残ります。自前の scrape config を書く場合も同じ設定にしてください — デフォルトの `honor_labels: false` ではラベルが `exported_namespace` にリネームされ、以下のクエリが壊れます。

## Grafana パネル

出発点として便利なクエリ(すべて `namespace`/`suite` でフィルタ可能):

```promql
# ステータステーブル / stat パネル: 今何が赤いか
verikube_check_last_result

# fail 中のチェック数
count(verikube_check_last_result == 0) OR on() vector(0)

# check ごとの失敗レート
sum by (namespace, suite, check)
  (rate(verikube_check_result_total{result="fail"}[$__rate_interval]))

# probe レイテンシ p50 / p95 / p99
histogram_quantile(0.95, sum by (namespace, suite, check, le)
  (rate(verikube_check_duration_seconds_bucket[$__rate_interval])))

# phase ごとの 1 時間あたり run 数(stacked)
sum by (phase) (increase(verikube_checkruns_total[1h]))

# スイートが最後に結果を出してからの秒数
time() - verikube_checkrun_last_completion_timestamp_seconds
```

ステータステーブルでは value mapping と color-background セル表示で値 `1` → PASS(緑)、`0` → FAIL(赤)にマップしてください。

## アラート例

```yaml
groups:
  - name: verikube
    rules:
      - alert: VerikubeCheckFailing
        expr: verikube_check_last_result == 0
        for: 10m
        labels: { severity: warning }
        annotations:
          summary: "Check {{ $labels.check }} in {{ $labels.namespace }}/{{ $labels.suite }} is failing"

      # 閾値は最長のスケジュール間隔の約 2 倍に。
      # 注意: この式は一度でも結果を出したスイートしかカバーしません
      # (gauge は初回の完了 run まで存在しません)。下の VerikubeRunErrors
      # は「ウィンドウ内に Error になった run」を検知します。一度も run が
      # 走っていないスイートはどちらにも映りません。
      - alert: VerikubeSuiteStale
        expr: time() - verikube_checkrun_last_completion_timestamp_seconds > 3600
        labels: { severity: warning }
        annotations:
          summary: "Suite {{ $labels.namespace }}/{{ $labels.suite }} has not completed a run with results in 1h"

      - alert: VerikubeRunErrors
        expr: increase(verikube_checkruns_total{phase="Error"}[1h]) > 0
        labels: { severity: warning }
        annotations:
          summary: "Runs for {{ $labels.namespace }}/{{ $labels.suite }} are erroring (no results produced)"
```
