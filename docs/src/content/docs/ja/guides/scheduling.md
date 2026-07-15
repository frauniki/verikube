---
title: スケジューリングとトリガー
description: cron スケジュール、run-now による手動トリガー、suspend、concurrencyPolicy、historyLimit。
---

スイートは cron スケジュール、手動トリガー、またはその両方で実行され
ます。

## cron スケジュール

`spec.schedule` は標準の 5 フィールド cron 式で、**UTC** で評価され
ます:

```yaml
spec:
  schedule: "*/30 * * * *"   # 30 分ごと
```

手動実行専用のスイートでは `schedule` を丸ごと省略します。

遅れた・逃した tick(operator の再起動、suspend 中のスイート)は
`startingDeadline`(デフォルト 200s)が制御します: deadline より古い
tick はスキップされるため、suspend の解除や operator の再起動で古い
キャッチアップ run が大量に発火することは**ありません**。

## 手動トリガー(run-now)

`verikube.dev/run-now` アノテーションの値を変えると 1 回の run が
トリガーされます — suspend 中でも動きます:

```bash
kubectl annotate checksuite payment-network \
  verikube.dev/run-now="$(date +%s)" --overwrite
```

前回処理された値と違いさえすればどんな値でも構いません(operator は
冪等性のため値を `status.lastManualTrigger` に記録します)。タイム
スタンプが自然な選択です。

## suspend

```yaml
spec:
  suspend: true
```

スイートを削除せずにスケジュール実行を止めます。suspend 中も手動
トリガーは動くので、一時停止したスイートのデバッグに便利です。

## concurrencyPolicy

`concurrencyPolicy` は、前の run がまだ実行中のときに新しい run を
どう扱うかを決めます:

| 値 | 挙動 |
|---|---|
| `Forbid`(デフォルト) | 実行中の run がある間は新しい run をスキップ |
| `Allow` | run の重複を許可 |
| `Replace` | 実行中の run を削除して新しい run を開始 |

`Allow` では重複した run が順不同に完了することがあります。
[状態メトリクス](/verikube/ja/guides/observability/)には最後に完了した
run が反映されます。

## historyLimit

完了した CheckRun はスイートごとにガベージコレクトされます:

```yaml
spec:
  historyLimit:
    successful: 3   # デフォルト 3
    failed: 5       # デフォルト 5。Error で終わった run も含む
```

`failed` は `successful` より多めに — インシデント時に読むのは失敗の
履歴です。

## timeout

`spec.timeout`(デフォルト 10m)は run 全体の deadline です。超過した
run は `DeadlineExceeded` condition 付きの `Error` になります。
