---
title: 運用ノート
description: タイムゾーンの挙動、missed tick の扱い、operator のアップグレード、バージョンスキュー。
---

VeriKube を本番で動かす前に知っておくべき事実。

## スケジュールは UTC で動く

cron 式は標準の 5 フィールド cron で、**UTC** で評価されます。スイート
ごとのタイムゾーン設定はありません。スケジュールを書くときに意図する
ローカル時間から変換してください。

## missed tick は破棄され、リプレイされない

`startingDeadline`(デフォルト 200s)は、逃した tick がどれだけ遅れて
発火してよいかを制限します。それより古いものはスキップされるため:

- スイートの suspend を解除しても、suspend していた期間のキャッチ
  アップ run は発火しません。
- operator を再起動しても、停止中に逃した tick の run が大量に発火する
  ことはありません。

スイートは単純に次の通常の tick から再開します。

## 実行中の run があっても operator のアップグレードは安全

すべての状態は Kubernetes API にあります: CheckRun の spec は不変な
スナップショット、結果は runner pod から server-side apply で届き、
runner Job は作成後は不変です。run の途中で operator pod を入れ替えても
何も失われず、新しい pod が集約を引き継ぎます。

## runner イメージのバージョンスキュー

runner pod はデフォルトで operator のイメージを使います。chart で
`runnerImage` を上書きした場合、古い runner は知らない check type を
黙ってスキップせず明示的な失敗(`unknown check type`)として報告します —
偽の pass ではなく失敗が見えます。特別な理由がなければ上書きしないで
ください。

## リソースフットプリント

operator のデフォルトは控えめな requests(CPU 50m / メモリ 64Mi、
limit 256Mi)です — chart の `resources` で調整してください。runner pod
は短命な Job pod で、数は `runners[].replicas` で制限されます(スイート
あたり最大 16 runner × 16 replicas)。

## 履歴の増加

完了した CheckRun は `historyLimit`(デフォルト: successful 3、
failed/error 5)によりスイートごとにガベージコレクトされます。etcd の
消費は稼働時間ではなくこの上限に比例します。
