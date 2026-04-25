# Runtime and Watch

## Resolved state

Jubako は型付きの最終設定を `Store[T]` として公開しますが、runtime model は意図的に分割されています。

- `resolved *Cell[T]` が現在の型付き値を持つ
- layer entry が layer ごとの data と dirty state を持つ
- origin index が provenance lookup 用の構造を持つ

この分離により、reload のたびに store オブジェクト全体を作り直さずに済みます。

## なぜ `Cell[T]` があるのか

`Cell[T]` は次を提供します。

- 現在の resolved value への lock-free read
- runtime container 自体の安定した所有
- resolved update に対する subscription hook

重要なのは、「中の値は変わっても、入れ物は変わらない」ことです。

これにより Jubako は、アプリケーション側の store 配線を組み直さずに hot reload を成立させます。

## Subscription model

subscription は個々の source ではなく、resolved runtime state にぶら下がります。

つまり subscriber が見るのは、次をすべて終えた後の結果です。

- 関係する layer の更新
- stabilization の収束
- path remapping と decode

これにより契約が「storage event」ではなく「アプリケーションが使う最終設定」に揃います。

## Watch model

`Store.Watch` は watch 可能な各 layer に watcher を作らせ、それらを起動し、結果を 1 本の debounced reload loop に合流させます。

重要な性質は次です。

- `WithNoWatch()` を付けた layer は store-level watch から除外される
- watcher result は先に layer data を更新する
- その後 store 全体を re-materialize する
- subscriber と `OnReload` は resolved update が成功した後にだけ呼ばれる

## なぜ watch を store が持つのか

watch を store が管理することで、runtime contract を 1 箇所で守れます。

- 複数 source の変更をまとめられる
- 1 回の reload で 1 つの一貫した resolved update を作れる
- error reporting を一貫させられる

layer 単位の watch だけだと、「何かが変わった」ことまでは分かっても、「全体として安定した resolved state に到達したか」は分かりません。
