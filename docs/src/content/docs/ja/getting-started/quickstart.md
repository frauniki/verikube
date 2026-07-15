---
title: クイックスタート
description: ローカルの kind クラスタで最初のネットワークチェックを 5 分で動かす。
---

VeriKube をソースからビルドし、ローカルの [kind](https://kind.sigs.k8s.io/) クラスタにインストールして、最初のチェックスイートを実行します。Docker・kind・kubectl・Helm が必要です。

:::note
VeriKube はまだ初回リリースを公開していないため、以下では operator のイメージと Helm chart をソースからビルドします。v0.1.0 以降はビルドを省略して直接インストールできるようになります:

```bash
helm install verikube oci://ghcr.io/frauniki/charts/verikube \
  --namespace verikube-system --create-namespace \
  --set checkNamespaces='{demo}'
```
:::

## 1. イメージをビルドしてクラスタを作成

```bash
git clone https://github.com/frauniki/verikube.git
cd verikube

make docker-build IMG=verikube:dev
kind create cluster --name verikube
kind load docker-image verikube:dev --name verikube
```

## 2. operator をインストール

`checkNamespaces` には CheckSuite を置くすべての namespace を列挙します — chart がそこに runner の ServiceAccount と RoleBinding を作成するため、namespace は先に存在している必要があります。ここでは `demo` namespace を使います:

```bash
kubectl create namespace demo

helm install verikube ./charts/verikube \
  --namespace verikube-system --create-namespace \
  --set image.repository=verikube \
  --set image.tag=dev \
  --set checkNamespaces='{demo}'
```

## 3. CheckSuite を作成

以下の 2 つのチェックはどんな新規クラスタでも動きます: 1 つ目は Kubernetes API service への probe、2 つ目はブラックホールになっている [TEST-NET](https://datatracker.ietf.org/doc/html/rfc5737) アドレスに **届かない**ことの検証(ネガティブテスト)です。

```yaml
# suite.yaml
apiVersion: verikube.dev/v1alpha1
kind: CheckSuite
metadata:
  name: hello
  namespace: demo
spec:
  runners:
    - name: default
  checks:
    - name: kube-api-reachable
      tcp:
        address: "kubernetes.default.svc:443"
    - name: blackhole-blocked
      tcp:
        address: "192.0.2.1:443"
        timeout: 2s
      expect: Failure
```

```bash
kubectl apply -f suite.yaml
```

`schedule` が無いのでまだ何も実行されません — このスイートは手動実行専用です。

## 4. run をトリガー

```bash
kubectl annotate checksuite hello -n demo \
  verikube.dev/run-now="$(date +%s)" --overwrite
```

operator が CheckRun を作成し、runner Job が両方のチェックを実行して、結果が CheckRun の status に入ります:

```bash
$ kubectl get checkrun -n demo
NAME               SUITE   PHASE       PASSED   FAILED   STARTED   AGE
hello-1784112900   hello   Succeeded   2        0        10s       10s
```

両方のチェックが pass しました — ネガティブテストは接続が失敗した *からこそ* pass しています。pod ごとの詳細は status にあります:

```bash
kubectl get checkrun -n demo -o yaml | grep -A 12 'runners:'
```

## 5. 後片付け

```bash
kind delete cluster --name verikube
```

## 次のステップ

- [インストール](/verikube/ja/getting-started/installation/) — chart のオプション、CRD のライフサイクル、メトリクス
- [コンセプト](/verikube/ja/concepts/) — CheckSuite・CheckRun・runner の関係
- [チェックを書く](/verikube/ja/guides/writing-checks/) — TCP・HTTP・ gRPC、リトライ、ネガティブテスト
- [スケジューリング](/verikube/ja/guides/scheduling/) — cron スケジュールと手動トリガー
