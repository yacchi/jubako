# Write Path

## 目的

write path は、「値を見えるようにしていた layer 境界を失わずに設定を更新する」ためのものです。

実際の書き込みは、「merge 後の結果にキーを set する」ことではありません。流れは次です。

1. 書き込み先 layer が妥当か検証する
2. その layer の論理 map を更新する
3. save 用の patch set を記録する
4. store を再 materialize する
5. 明示的に save が呼ばれたときだけ永続化する

## なぜ layer を指定して書くのか

Jubako は `SetTo` のような mutation API で layer を明示させます。

この制約によって次が守られます。

- 書き込み先の ownership が明示される
- 物理 store への正しい write-back ができる
- 実効値と保存場所を分離できる

layer 指定がない merged tree だけでは、「この変更をどこへ保存すべきか」に安全に答えられません。

## Validation

layer を変更する前に、store は次を検証します。

- store が load 済みであること
- 対象 layer が存在すること
- layer が writable で read-only ではないこと
- path が妥当であること
- sensitive rule に違反しないこと

sensitive は「どこへ書いてよいか」の guardrail として扱います。外部 store への routing そのものを表す仕組みではありません。

## Dirty state

Jubako は dirty 状態を 2 段階で持ちます。

- その layer に未保存の論理変更がある
- store 全体として dirty な layer が 1 つ以上ある

save は layer 単位なので、この分離が重要です。1 つの layer しか変わっていないのに全体を書き直す必要はありません。

また snapshot-aware layer は、論理値そのものを直接変更していなくても、物理 projection が古くなったことで dirty になれます。

## Save model

save 時、Jubako は次の両方を満たす layer だけを永続化します。

- save をサポートしている
- 未保存 change または projection work がある

通常の document-backed layer では、loaded bytes に対して patch set を適用し、それを source に渡して保存します。

contextual layer では、store はより広い save context を渡します。layer はそれを使って次を見ながら保存できます。

- 現在の論理状態
- patch 適用後の論理状態
- schema metadata

これにより、1 つの論理変更を複数の物理書き込みへ展開する coordinated layer が可能になります。
