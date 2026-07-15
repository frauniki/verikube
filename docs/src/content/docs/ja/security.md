---
title: セキュリティモデル
description: 信頼境界、runner pod の権限、ローカルターゲットの遮断、残余リスク。
---

## 信頼境界は CheckSuite/CheckRun の create 権限

CheckSuite(または CheckRun を直接。ad hoc 実行)を作成できる者は、任意の場所に配置した pod から任意のアドレスを probe できます — **それがこのツールの目的です**(唯一の例外はデフォルトで遮断されるローカルターゲット。後述)。suite の spec 内にターゲットの許可リストはありません。制御するのは「誰が・どの namespace で」スイートを作成できるかです。

CRD のロールは namespace ごとに、意図をもって付与してください:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: verikube-editor
  namespace: payment
rules:
  - apiGroups: ["verikube.dev"]
    resources: ["checksuites", "checkruns"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## runner pod の権限

runner pod は意図的に退屈な作りです:

- 固定のイメージとコマンド、追加の権限なし
- ServiceAccount は `get checkruns` と `patch checkruns/status` のみ — 結果を報告するのに必要な分だけ

## ローカルターゲットはデフォルトで遮断

loopback / link-local アドレス — cloud metadata の `169.254.169.254` を含む — を対象とするチェックは**デフォルトで拒否** されます。チェックをノードや cloud control plane への SSRF 的な probe に使わせないためのガードです。そうしたターゲットの probe が本当に意図的なら、chart の values で `allowLocalTargets: true` にしてください。

## 残余リスク: check namespace 内での status 書き込み

`checkNamespaces` に列挙された namespace 内では、pod-create 権限を持つものは runner の ServiceAccount をマウントし、そこにあるあらゆる CheckRun の status を patch できます。これは防げません(runner が報告するために ServiceAccount は存在せざるを得ません)が、検知はします: controller は自身の Job と pod 名が一致しない結果エントリに対して `ForeignResultEntry` warning イベントを発行します。

check namespace での pod-create 権限は「チェック結果を書ける」ことを意味すると考え、`checkNamespaces` のスコープを絞ってください。

## 報告

セキュリティ上の問題を見つけた場合は、公開 issue ではなく [GitHub security advisories](https://github.com/frauniki/verikube/security/advisories/new)から非公開で報告してください。
