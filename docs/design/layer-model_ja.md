# Layer Model

## 目的

Jubako は、「最終的な設定が解決された後でも、設定ソースをレイヤーとして見えるままに保つ」ことを前提に設計しています。

多くの設定システムは「読んで merge する」ところで役割を終えますが、Jubako はその先の問いに答えられるように、構造を残します。

- いま効いている値はどの layer から来たのか
- どの layer に書き戻すべきか
- その layer は watch できるか
- その layer は optional / sensitive / read-only か

## 中核となる抽象

### `Source`

`Source` は I/O を担当します。raw bytes を読み込み、書き込み可能な場合は optimistic locking 的な更新手順で保存します。

### `Document`

`Document` はフォーマットの意味論を担当します。bytes を `map[string]any` に変換し、対応フォーマットでは元の bytes に対して
patch set を適用して更新できます。全体再生成を必須にしません。

### `Layer`

`Layer` は `Store` から見える論理単位です。`map[string]any` をロードし、必要なら change set を保存し、必要なら watch を持ちます。

この境界を明確にしているのが重要です。

- `Source` は保存先
- `Document` は構文と patch 適用
- `Layer` は論理設定の境界

## なぜ `Store` は layer を相手にするのか

`Store` は source 単位でも document 単位でもなく、layer 単位で振る舞います。

ここで layer を潰してしまうと、次の性質を保ちにくくなります。

- per-layer metadata
- origin tracking
- save routing
- watch behavior
- 1 つの論理 layer を複数の物理 store に投影する coordinated layer

## Layer metadata

layer を `Add` するとき、store はその layer に対して次のポリシーと実行時属性を持ちます。

- priority
- read-only
- no-watch
- sensitive
- optional

これらは source 自体の能力とは別の、store 側の参加ルールです。たとえば source が書き込み可能でも、store では read-only として
扱えます。

## Schema-aware extension

store は型 `T` から schema 情報を一度構築し、必要な layer には read-only な `SchemaView` を渡します。

これにより layer は次を参照できます。

- 解決済み path
- sensitive 指定
- 生の struct tag
- `reflect.StructField`

重要なのは、これはあくまで read-only であることです。schema の構築と所有権は store 側に残し、layer は参照だけ行います。
