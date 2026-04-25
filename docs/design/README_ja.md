# Jubako 設計資料

このディレクトリには、Jubako の恒久的な設計メモを置きます。

`plans/` とは役割を分けます。

- `docs/design/` は、現時点での設計モデルとその理由を残す場所
- `plans/` は、探索中の案、実装計画、未確定の検討を置く場所

このリポジトリでは日本語版を主参照として扱い、英語版は将来の公開ドキュメントや README 連携を見据えて同じ構成で併置します。

## 文書一覧

- [layer-model_ja.md](layer-model_ja.md): 中核となる抽象とレイヤー境界
- [materialization_ja.md](materialization_ja.md): merge、schema mapping、origin tracking
- [write-path_ja.md](write-path_ja.md): `SetTo`、dirty tracking、save の流れ
- [runtime-and-watch_ja.md](runtime-and-watch_ja.md): resolved state、購読、watch
- [coordinated-layer_ja.md](coordinated-layer_ja.md): 1 つの論理レイヤーを複数の物理ストアで支える設計

## 読む順番

1. まず [layer-model_ja.md](layer-model_ja.md)
2. 次に [materialization_ja.md](materialization_ja.md)
3. 変更対象に応じて [write-path_ja.md](write-path_ja.md) と [runtime-and-watch_ja.md](runtime-and-watch_ja.md)
4. 外部 secret や multi-store projection を触るときは [coordinated-layer_ja.md](coordinated-layer_ja.md)
