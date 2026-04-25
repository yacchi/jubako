# Coordinated Layer

## 問題設定

設定値の中には、ちょうど 1 つの物理 store にだけ存在するとは限らないものがあります。

典型例は credentials です。

- 人が見える document 側の項目は document layer に残したい
- secret 本体は安全な外部 store に置きたい
- それでもアプリケーションからは 1 つの論理設定として扱いたい

この分岐をアプリケーションコードに直接埋め込むと、分岐が各所に漏れやすくなります。Jubako の coordinated layer は、その調停を
「論理 layer の境界の内側」に閉じ込めるための設計です。

## 設計の考え方

coordinated layer は store から見ると 1 つの layer のままですが、内部では複数 collaborator を使って状態を再構成し、保存できます。

抽象は次のように変わります。

- 1 つの path が 1 つの backend に行く

から、

- 1 つの論理 read が複数の物理 store から hydrate される
- 1 つの論理 write が複数の物理 write に分岐する

実際には、helper が 1 つの entry を次の 3 つの状態として扱うと理解しやすくなります。

1. current physical state
2. current logical state
3. target physical state

current physical state には、document 側に残る項目に加えて `secret_ref` で参照される既存の external payload が含まれることがあります。
helper はまず current logical state を復元し、その後で target physical state へ再投影する、という順序で考えるのが自然です。

## store 側に必要な支援

これを無理なく支えるため、store は狭い read-only context を渡します。

- path と field metadata のための `SchemaView`
- 暫定的な論理 snapshot のための `StabilizeContext`
- before/after の論理状態を見るための `SaveContext`

orchestration の主導権は store が持ち、coordinated layer は論理状態をどのように backing store へ投影するかを決めます。

## なぜ stabilization が必要か

coordinated layer は、最終的な resolved state を信用できるようにする前段階として、明示的な stabilization を必要とすることがあります。

典型例は次です。

- plaintext credential を secure store へ migrate する
- `storage:"keyring"` が付いた field を secret reference から hydrate する
- 新しい route が local を向いていても、既存 external payload から一度 logical state を復元する
- 物理 projection が policy とズレたので subtree を dirty にする

## 意図している境界

`storage:"keyring"` のような tag の意味を Jubako core が定義する必要はありません。

Jubako が提供すべきなのは、アプリケーション固有の coordinator がその tag を解釈し、ドメイン固有の load/save を実装できるだけの
schema visibility です。

この境界にしておくことで、core は汎用のまま保ちつつ、外部化された設定の扱いも現実的にできます。

## Route の解釈

`external-store.NewMap` のような helper における `RouteForEntry` は、
「この entry は今どこへ投影されるべきか」を返すものとして読むのが適切です。
「この entry が現在 external backing を持っているか」を直接表すものではありません。

この区別は双方向 migration で重要です。

- `local -> external`: current physical state は document 側にある plaintext の項目、target は external
- `external -> local`: `ExistingRef` が指す external payload は current physical state の一部であり、target route が local でも一度 logical state に戻す必要がある

このため、`ExistingRef` がある場合は `UseExternal == false` でも stable な `Ref` を返す必要があります。
`ExternalKey` を省略した場合、helper は `Ref` をそのまま backend key として使います。

## `Ref` と `ExternalKey` の違い

`Ref` と `ExternalKey` は関係していますが、常に別物というわけではありません。

| 項目            | 意味                                       | 主な保存先/用途                                              | 安定性の期待                        |
|---------------|------------------------------------------|-------------------------------------------------------|-------------------------------|
| `Ref`         | document 側の表現に残る論理参照                     | document layer の `secret_ref` など                      | 論理的な紐付けが続く間は安定しているべき          |
| `ExternalKey` | external backend に対して実際に使う物理キーの override | keyring, secret manager, 外部 store への `Get/Set/Delete` | 指定する場合は通常 ref の寿命に対して安定しているべき |

言い換えると、次の違いです。

- `Ref` は document 側の表現が覚えておく参照
- `ExternalKey` は external backend に問い合わせるための実キー override

基本ルールは単純です。

- `ExternalKey` が空なら、helper は `Ref` をそのまま使う

多くのケースでは、このデフォルトで十分です。

例えば 1 つの credential entry を keyring へ移行した後は、次のようになります。

- document layer には `secret_ref: "alice@example-space"` が残る
- backend に対しても `"alice@example-space"` をそのまま keyring key として使う

この例では、

- `alice@example-space` が logical attachment identity
- そしてデフォルトでは、そのまま concrete な backend key にもなる

別の `ExternalKey` が必要になるのは、たとえば次のようなケースです。

- backend ごとの prefix や namespace を付けたい
- backend の文字種・長さ制限に合わせて変換したい
- document 側には短い stable ref を残しつつ、external 側では別の物理キーを使いたい

別の `ExternalKey` を使う場合に限って、次の 2 つの問いを分けて考える必要があります。

- `Ref`: この論理 entry が後で external state を参照できるよう、document 側の表現には何を残すべきか
- `ExternalKey`: 今この瞬間に backend へ問い合わせるとき、どのキーを使うべきか

`ExistingRef` がある場合、`RouteForEntry` は同じ `Ref` を返し続ける必要があります。  
`ExternalKey` を省略しているなら、helper は引き続きその `Ref` を backend key として使います。  
別の `ExternalKey` を使う場合は、target route が local に変わっていても、「すでに紐付いている external state」に対応する `ExternalKey` を返す必要があります。

## Route 設計ガイドライン

可能であれば、次の方針を取るのが安全です。

- `Ref` は安定した論理 identity から作る
- `ExternalKey` は省略し、`Ref` をそのまま使う

別の `ExternalKey` を使うなら、mutable な routing input ではなく stable ref から作る方が安全です。

tenant ID、domain、space identifier のような mutable な値を override した `ExternalKey`
に含めると、entry が external のままでも物理キーだけが drift する可能性があります。
現在の helper は、`UseExternal == true` のまま `ExternalKey` だけが変わるケースを
自動で再投影しません。

現時点では、利用側の安全なルールとして

- `ExternalKey` は ref の寿命に対して安定に保つ

と考えるのがよいです。

mutable な external key を本格的に扱いたい場合は、明示的な reprojection
support を持つ将来拡張として扱う方がよいです。

## Partial failure semantics

coordinated save は現在、best-effort な協調と明示的な error propagation を提供します。

次のような複数の物理操作をまたいだ rollback は提供しません。

- external `Set`
- external `Delete`
- document layer の `Save`

そのため、途中で失敗した場合には、一部の物理更新だけが適用され、残りは未適用という状態が起こりえます。
より強い transactional guarantee が必要であれば、それは別の設計改善として扱うべきです。
