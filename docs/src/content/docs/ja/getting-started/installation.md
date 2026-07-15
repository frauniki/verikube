---
title: インストール
description: Helm での VeriKube operator のインストール — namespace、CRD のライフサイクル、chart オプション。
---

VeriKube は Helm でインストールします。chart は operator・CRD・runner
pod に必要な RBAC をデプロイします。

```bash
helm install verikube oci://ghcr.io/frauniki/charts/verikube \
  --namespace verikube-system --create-namespace \
  --set checkNamespaces='{payment,batch}'
```

:::note
初回リリースが公開されるまでは、ローカルの checkout からインストール
してください: OCI 参照の代わりに `./charts/verikube` を使い、イメージの
ビルド手順は[クイックスタート](/verikube/ja/getting-started/quickstart/)
を参照してください。
:::

## check namespace

`checkNamespaces` には(リリース namespace に加えて)CheckSuite を置く
すべての namespace を列挙します。chart がそれぞれに runner の
ServiceAccount と RoleBinding を用意します。

列挙されて**いない** namespace に作られた CheckSuite は即座に失敗します:
run に `RunnerServiceAccountMissing` condition が付き、どこにどの
ServiceAccount が足りないかを正確に教えてくれます。あとから namespace を
追加するには:

```bash
helm upgrade verikube oci://ghcr.io/frauniki/charts/verikube --reuse-values \
  --set checkNamespaces='{payment,batch,newteam}'
```

## CRD のライフサイクル

CRD は Helm 特有の `crds/` ディレクトリではなく、テンプレート化された
chart リソースとしてインストールされます。その帰結が 2 つ:

- **`helm upgrade` がスキーマ変更を反映します。** CRD の更新を手で
  apply する必要はありません。
- **`helm uninstall` してもデータは残ります。** CRD にはデフォルトで
  `helm.sh/resource-policy: keep` が付く(`crds.keep=true`)ため、chart を
  アンインストールしても CheckSuite や run の履歴は消えません。

CRD を別の仕組みで管理している場合は `crds.enabled=false` にしてください。

## runner イメージ

runner Job の pod はデフォルトで operator と同じイメージを使います
(`runnerImage: ""`)。`runnerImage` の上書きにはバージョンスキューの
リスクがあります: インストール済み CRD より古い runner は、知らない
check type を黙ってスキップせず明示的な失敗(`unknown check type`)として
報告します。特別な理由がなければ空のままにしてください。

## メトリクス

operator はチェック結果を Prometheus メトリクスとして公開できます:

```bash
helm upgrade verikube oci://ghcr.io/frauniki/charts/verikube --reuse-values \
  --set metrics.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.reader.serviceAccountName=prometheus-kube-prometheus-prometheus \
  --set metrics.reader.namespace=monitoring
```

エンドポイントは認証・認可付きの HTTPS で提供されます。
`metrics.reader.*` は chart 付属の ClusterRole をあなたの Prometheus の
ServiceAccount にバインドし、scrape を可能にします。詳しくは
[Observability](/verikube/ja/guides/observability/) を参照してください。

## セキュリティ関連の値

| 値 | デフォルト | 意味 |
|---|---|---|
| `allowLocalTargets` | `false` | loopback / link-local アドレス(cloud metadata の `169.254.169.254` など)を対象とするチェックは拒否されます。`true` で許可。 |
| `runnerServiceAccount` | `verikube-runner` | runner pod の ServiceAccount 名。各 check namespace に作成されます。 |

CheckSuite の権限を付与する前に
[セキュリティモデル](/verikube/ja/security/)を読んでください。

## chart values リファレンス

すべてのオプションは chart の
[values.yaml](https://github.com/frauniki/verikube/blob/main/charts/verikube/values.yaml)
にあり、それぞれコメントで説明されています。
