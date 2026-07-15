---
title: チェックを書く
description: TCP・HTTP・gRPC の probe、リトライ、expect Failure によるネガティブテスト。
---

チェックはスイートの `checks` リストの 1 エントリです: 名前、ちょうど
1 つの probe type(`tcp`・`http`・`grpc`)、そしてリトライや期待結果と
いった省略可能な挙動からなります。すべてを組み合わせた例:

```yaml
apiVersion: verikube.dev/v1alpha1
kind: CheckSuite
metadata:
  name: payment-network
  namespace: payment
spec:
  schedule: "*/30 * * * *"     # UTC。手動実行専用なら省略
  runners:
    - name: payment-nodes
      replicas: 3
      nodeSelector: { payment-ng: "true" }
      topologySpread:
        topologyKey: topology.kubernetes.io/zone
    - name: batch-nodes
      nodeSelector: { batch-ng: "true" }
  checks:
    - name: db-reachable               # すべての runner から
      tcp: { address: "db.internal:3306", timeout: 5s }
    - name: api-health                 # payment ノードからのみ
      runners: [payment-nodes]
      http:
        url: "https://api.internal/health"
        headers:
          - { name: Host, value: api.example.com }
        expectedStatus: [200]
      retries: { attempts: 3, delay: 5s }
    - name: payments-grpc              # gRPC Health Checking Protocol
      grpc:
        address: "payments.internal:50051"
        service: payments.v1.Payments  # 省略でサーバー全体の health
    - name: external-blocked           # ネガティブテスト: 繋がってはいけない
      tcp: { address: "blocked.example.com:443" }
      expect: Failure
```

## TCP

TCP チェックは接続を確立できたら pass します:

```yaml
- name: db-reachable
  tcp:
    address: "db.internal:3306"   # host:port、スキームなし
    timeout: 5s                   # dial のタイムアウト、デフォルト 1s
```

## HTTP

HTTP チェックはリクエストを実行してレスポンスステータスを検証します:

```yaml
- name: api-health
  http:
    url: "https://api.internal/health"
    method: GET                   # デフォルト GET
    headers:
      - { name: Host, value: api.example.com }
    expectedStatus: [200, 204]    # デフォルト [200]
    timeout: 10s                  # リクエスト全体、デフォルト 30s
```

`Host` ヘッダーはリクエストのホストを上書きします — URL では LB の
アドレスを直接指定しつつ、ホスト名でルーティングするロードバランサー
越しに probe したいときに便利です。

## gRPC

gRPC チェックは標準の
[gRPC Health Checking Protocol](https://grpc.io/docs/guides/health-checking/)
(`grpc.health.v1.Health/Check`)を使い、サーバーが `SERVING` を報告したら
pass します:

```yaml
- name: payments-grpc
  grpc:
    address: "payments.internal:50051"
    service: payments.v1.Payments   # 省略でサーバー全体の health
    timeout: 5s                     # デフォルト 5s
    tls: {}                         # 証明書検証ありの TLS。省略で平文
```

自己署名証明書の内部エンドポイント向けに `tls` は
`insecureSkipVerify: true` も受け付けますが、可能なら CA を正しく信頼
させる方を選んでください。

## ネガティブテスト

`expect: Failure` は verdict を反転させます: **probe が失敗したときに
pass** します。「ファイアウォールはこれを遮断しているはず」を思い込み
から継続的に検証されるアサーションに変えます:

```yaml
- name: metadata-blocked
  tcp: { address: "internal-only.example.com:443", timeout: 2s }
  expect: Failure
```

生の probe 結果は result に保存される(`observed: Success|Failure`)ため、
fail したネガティブテストからは「繋がるべきでないのに接続が*成功した*」
ことが読み取れます。

## リトライ

`retries` は観測結果が期待と一致しなかったチェックを再実行します。最後の
試行の結果が報告され、試行回数も result に記録されます:

```yaml
- name: flaky-endpoint
  http: { url: "https://api.internal/health" }
  retries:
    attempts: 3   # 初回を含む合計。最大 10
    delay: 5s     # 試行間隔、デフォルト 1s
```

## チェックの実行元を限定する

デフォルトではすべてのチェックがすべての runner から実行されます。
`checks[].runners` でサブセットを指定できます:

```yaml
runners:
  - name: payment-nodes
    nodeSelector: { payment-ng: "true" }
  - name: batch-nodes
    nodeSelector: { batch-ng: "true" }
checks:
  - name: api-health
    runners: [payment-nodes]     # 上で定義した名前を参照すること
    http: { url: "https://api.internal/health" }
```

チェック全体としての pass は、それを実行した**すべての pod** で pass
した場合のみです — `replicas: 3` なら 1 台のノードの異常でチェックが
fail します。複数の場所から実行する意義はまさにそこにあります。

すべてのフィールド・デフォルト値・バリデーションルールは
[API リファレンス](/verikube/ja/reference/api/)に記載されています。
