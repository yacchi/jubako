# Materialization

## 役割

materialization は、layer ごとの `map[string]any` から最終的な型付き設定値を作る段階です。

これは単なる merge ではありません。Jubako では materialization に次の関心事が集約されます。

- layer ordering
- snapshot-aware layer の stabilization
- origin tracking
- path remapping
- 型 `T` への decode

## パイプライン

現在の流れは次のとおりです。

1. priority に従って layer を並べる
2. `SnapshotAwareLayer` を持つ layer に対して stabilization pass を回す
3. layer data を 1 つの論理 map に merge する
4. 収束後の layer data から origin index を再構築する
5. schema ベースの path mapping を適用する
6. 型 `T` に decode する
7. `Cell[T]` に resolved value を publish する

## Stabilization

stabilization は、「その layer の論理データが、自分自身の storage だけでは決まらず、より広い snapshot に依存する layer」のためにあります。

典型例は次です。

- file metadata と外部 secret store から 1 つの論理値を再構成する coordinated layer
- 他の resolved path によって物理投影先が変わる layer

store は layer 群が収束するまで pass を回します。また fingerprint を持って、発振していないかを検出します。

## Merge semantics

現在のデフォルト merge ルールは次です。

- map は再帰的に merge
- map 以外は後勝ちで置き換え

この形にしておくと、挙動が説明しやすく、予測もしやすくなります。将来的に field ごとの高度な merge 戦略を入れても、
materialization の骨格自体は変えずに済みます。

## Origin tracking

stabilization の後、store は各 layer の確定済み data を walk して、次を記録します。

- leaf origin
- container origin

これにより、merge 後でも `GetAt`、`GetAllAt`、`Walk` が provenance を返せます。

重要なのは、origin は「merge 済み結果から逆算する」のではなく、「layer data から再構築する」ことです。

## Path remapping と decode

型 `T` に decode する前に、Jubako は `jubako` tag と type conversion rule に基づいて schema mapping を適用します。

ここでは関心事を分けています。

- merge は論理 map 空間で動く
- decode は型指向の schema 空間で動く

この分離により、merge engine は汎用のまま保ちつつ、schema-driven なアクセスや変換も実現できます。
