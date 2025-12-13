# Jubako

**Jubako**（重箱）は、Go 言語向けのレイヤー型設定管理ライブラリです。

複数の設定ソースをレイヤーとして扱い、優先度に従って最終的な設定を解決します。名前は重箱にちなんでいます。

[English README](README.md)

## 目次

- [特徴](#特徴)
- [インストール](#インストール)
- [クイックスタート](#クイックスタート)
- [コアコンセプト](#コアコンセプト)
    - [レイヤー](#レイヤー)
    - [JSON Pointer (RFC 6901)](#json-pointer-rfc-6901)
    - [設定構造体の定義](#設定構造体の定義)
- [API リファレンス](#api-リファレンス)
    - [Store[T]](#storet)
    - [オリジン追跡](#オリジン追跡)
    - [レイヤー情報](#レイヤー情報)
- [サポートされるフォーマット](#サポートされるフォーマット)
    - [環境変数レイヤー](#環境変数レイヤー)
- [独自フォーマット・ソースの作成](#独自フォーマットソースの作成)
    - [Source インターフェース](#source-インターフェース)
    - [Parser インターフェース](#parser-インターフェース)
    - [Document インターフェース](#document-インターフェース)
    - [mapdoc による簡易実装](#mapdoc-による簡易実装)
    - [Layer インターフェース](#layer-インターフェース)
- [一般的な設定ライブラリとの比較](#一般的な設定ライブラリとの比較)
- [ライセンス](#ライセンス)
- [コントリビューション](#コントリビューション)

## 特徴

- **レイヤー対応の設定管理** - 優先度順序付きで複数の設定ソースを管理
- **オリジン追跡** - 各設定値がどのレイヤーから来たか追跡可能
- **フォーマット保持** - AST ベースの処理により変更箇所のみを更新（コメント・空行・インデント等を維持）
- **型安全なアクセス** - ジェネリクスベースの API でコンパイル時の型チェック
- **変更通知** - 設定変更をサブスクライブ可能

## インストール

```bash
go get github.com/yacchi/jubako
```

**動作要件:** Go 1.24 以上

## クイックスタート

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/env"
	"github.com/yacchi/jubako/source/bytes"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
}

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type DatabaseConfig struct {
	URL string `yaml:"url" json:"url"`
}

const defaultsYAML = `
server:
  host: localhost
  port: 8080
database:
  url: postgres://localhost/myapp
`

func main() {
	ctx := context.Background()

	// 新しいストアを作成
	store := jubako.New[AppConfig]()

	// 設定レイヤーを追加（優先度の低い順）
	store.Add(
		layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
	)

	store.Add(
		layer.New("user", fs.New("~/.config/app/config.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityUser),
	)

	store.Add(
		layer.New("project", fs.New(".app.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityProject),
	)

	store.Add(
		env.New("env", "APP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	)

	// 設定を読み込みマテリアライズ
	if err := store.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// 解決済みの設定を取得
	config := store.Get()
	fmt.Printf("Server: %s:%d\n", config.Server.Host, config.Server.Port)

	// 変更をサブスクライブ
	unsubscribe := store.Subscribe(func(cfg AppConfig) {
		log.Printf("Config changed: %+v", cfg)
	})
	defer unsubscribe()
}
```

完全な動作例は [examples/](examples/) ディレクトリを参照してください：

- [basic](examples/basic/) - 基本的な使い方（レイヤー追加、読み込み、変更、保存）
- [env-override](examples/env-override/) - 環境変数による設定上書き
- [origin-tracking](examples/origin-tracking/) - オリジン追跡機能の詳細

## コアコンセプト

### レイヤー

各設定ソースは優先度を持つレイヤーとして表現されます。優先度の高いレイヤーが低いレイヤーの値を上書きします。

```go
const (
PriorityDefaults LayerPriority = 0  // 最低 - デフォルト値
PriorityUser     LayerPriority = 10 // ユーザーレベル設定 (~/.config)
PriorityProject  LayerPriority = 20 // プロジェクトレベル設定 (.app.yaml)
PriorityEnv      LayerPriority = 30 // 環境変数
PriorityFlags    LayerPriority = 40 // 最高 - コマンドラインフラグ
)
```

**優先度順序の例:**

```
┌─────────────────────┐
│   コマンドフラグ      │ ← 優先度 40（最高）
├─────────────────────┤
│   環境変数           │ ← 優先度 30
├─────────────────────┤
│   プロジェクト設定     │ ← 優先度 20
├─────────────────────┤
│   ユーザー設定        │ ← 優先度 10
├─────────────────────┤
│   デフォルト          │ ← 優先度 0（最低）
└─────────────────────┘
```

### JSON Pointer (RFC 6901)

Jubako はパスベースの設定アクセスに JSON Pointer を使用します：

```go
import "github.com/yacchi/jubako/jsonptr"

// ポインタを構築
ptr := jsonptr.Build("server", "port") // "/server/port"
ptr := jsonptr.Build("servers", 0, "name") // "/servers/0/name"

// ポインタを解析
keys, _ := jsonptr.Parse("/server/port") // ["server", "port"]

// 特殊文字の処理
ptr := jsonptr.Build("feature.flags", "on/off") // "/feature.flags/on~1off"
```

**エスケープルール（RFC 6901）：**

- `~` は `~0` としてエンコード
- `/` は `~1` としてエンコード

### 設定構造体の定義

設定構造体を定義する際は、`yaml` と `json` の両方のタグを指定してください。
マテリアライズ処理では内部的に JSON を使用してマージ済みのマップを構造体にデコードするため、`json` タグが必要です。

```go
type AppConfig struct {
Server   ServerConfig   `yaml:"server" json:"server"`
Database DatabaseConfig `yaml:"database" json:"database"`
}

type ServerConfig struct {
Host string `yaml:"host" json:"host"`
Port int    `yaml:"port" json:"port"`
}
```

## API リファレンス

### Store[T]

Store は設定管理の中心となる型です。

#### 作成と設定

```go
// 新しいストアを作成
store := jubako.New[AppConfig]()

// 自動優先度のステップを指定（デフォルト: 10）
store := jubako.New[AppConfig](jubako.WithPriorityStep(100))
```

#### レイヤーの追加

```go
// 優先度を指定してレイヤーを追加
err := store.Add(
layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
jubako.WithPriority(jubako.PriorityDefaults),
)

// 読み取り専用として追加（SetTo による変更を禁止）
err := store.Add(
layer.New("system", fs.New("/etc/app/config.yaml"), yaml.NewParser()),
jubako.WithPriority(jubako.PriorityDefaults),
jubako.WithReadOnly(),
)

// 優先度を省略すると追加順に自動割り当て（0, 10, 20, ...）
store.Add(layer.New("base", bytes.FromString(baseYAML), yaml.NewParser()))
store.Add(layer.New("override", bytes.FromString(overrideYAML), yaml.NewParser()))
```

#### 読み込みとアクセス

```go
// 全レイヤーを読み込み
err := store.Load(ctx)

// 設定をリロード
err := store.Reload(ctx)

// マージ済み設定を取得
config := store.Get()
fmt.Println(config.Server.Port)
```

#### 変更通知

```go
// 設定変更をサブスクライブ
unsubscribe := store.Subscribe(func (cfg AppConfig) {
log.Printf("Config changed: %+v", cfg)
})
defer unsubscribe()
```

#### 値の変更と保存

```go
// 特定レイヤーの値を変更（メモリ上）
err := store.SetTo("user", "/server/port", 9000)

// 変更があるか確認
if store.IsDirty() {
// 変更された全レイヤーを保存
err := store.Save(ctx)

// または特定レイヤーのみ保存
err := store.SaveLayer(ctx, "user")
}
```

### オリジン追跡

各設定値がどのレイヤーから来たかを追跡できます。

#### GetAt - 単一値の取得

```go
rv := store.GetAt("/server/port")
if rv.Exists {
fmt.Printf("port=%v (from layer %s)\n", rv.Value, rv.Layer.Name())
}
```

#### GetAllAt - 全レイヤーの値を取得

```go
values := store.GetAllAt("/server/port")
for _, rv := range values {
fmt.Printf("port=%v (from layer %s, priority %d)\n",
rv.Value, rv.Layer.Name(), rv.Layer.Priority())
}

// 最も優先度の高い値を取得
effective := values.Effective()
fmt.Printf("effective: %v\n", effective.Value)
```

#### Walk - 全設定値を走査

```go
// 各パスの解決済み値を取得
store.Walk(func (ctx jubako.WalkContext) bool {
rv := ctx.Value()
fmt.Printf("%s = %v (from %s)\n", ctx.Path, rv.Value, rv.Layer.Name())
return true // 継続
})

// 各パスの全レイヤー値を取得（オーバーライドチェーンの分析）
store.Walk(func (ctx jubako.WalkContext) bool {
allValues := ctx.AllValues()
if allValues.Len() > 1 {
fmt.Printf("%s has values from %d layers:\n", ctx.Path, allValues.Len())
for _, rv := range allValues {
fmt.Printf("  - %s: %v\n", rv.Layer.Name(), rv.Value)
}
}
return true
})
```

詳しい使用例は [examples/origin-tracking](examples/origin-tracking/) を参照してください。

### レイヤー情報

```go
// 特定レイヤーの情報を取得
info := store.GetLayerInfo("user")
if info != nil {
fmt.Printf("Name: %s\n", info.Name())
fmt.Printf("Priority: %d\n", info.Priority())
fmt.Printf("Format: %s\n", info.Format())
fmt.Printf("Path: %s\n", info.Path()) // ファイルベースの場合
fmt.Printf("Loaded: %v\n", info.Loaded())
fmt.Printf("ReadOnly: %v\n", info.ReadOnly())
fmt.Printf("Writable: %v\n", info.Writable())
fmt.Printf("Dirty: %v\n", info.Dirty())
}

// 全レイヤーを一覧（優先度順）
for _, info := range store.ListLayers() {
fmt.Printf("[%d] %s (writable: %v)\n",
info.Priority(), info.Name(), info.Writable())
}
```

## サポートされるフォーマット

Jubako は2種類のフォーマット実装をサポートしています：

### フルサポートフォーマット（フォーマット保持）

AST（抽象構文木）を直接操作することで、変更箇所のみを更新し、
コメント・空行・インデント・キーの順序などの元のフォーマットを保持したまま設定を編集・保存できます。

| フォーマット | パッケージ          | 説明                                     |
|--------|----------------|----------------------------------------|
| YAML   | `format/yaml`  | `gopkg.in/yaml.v3` の yaml.Node AST を使用 |
| TOML   | `format/toml`  | コメント/フォーマットを保持したまま編集・保存可能             |
| JSONC  | `format/jsonc` | コメント/フォーマットを保持したまま編集・保存可能             |

```go
// YAML（コメント保持）
store.Add(
layer.New("user", fs.New("~/.config/app.yaml"), yaml.NewParser()),
jubako.WithPriority(jubako.PriorityUser),
)
```

フルサポートフォーマットでは、値を変更しても元のフォーマットが維持されます：

```yaml
# ユーザー設定

server:
  port: 8080  # カスタムポート

# ↑ store.SetTo("user", "/server/port", 9000) を実行しても
# コメント・空行・インデントはそのまま維持される
```

### 簡易サポートフォーマット（mapdoc ベース）

`map[string]any` をバックエンドとする簡易実装です。フォーマットは保持されませんが、
読み書きは正常に動作します。

| フォーマット | パッケージ         | 説明                          |
|--------|---------------|-----------------------------|
| JSON   | `format/json` | 標準ライブラリ `encoding/json` を使用 |

```go
// JSON（コメント非保持）
store.Add(
layer.New("config", fs.New("config.json"), json.NewParser()),
jubako.WithPriority(jubako.PriorityProject),
)
```

### 一覧

| Source                | 追加方法                                          | Format 保持 |
|-----------------------|-----------------------------------------------|-----------|
| YAML                  | `layer.New(..., <source>, yaml.NewParser())`  | Yes       |
| TOML                  | `layer.New(..., <source>, toml.NewParser())`  | Yes       |
| JSONC                 | `layer.New(..., <source>, jsonc.NewParser())` | Yes       |
| JSON                  | `layer.New(..., <source>, json.NewParser())`  | No        |
| Environment variables | `env.New(name, prefix)`                       | N/A       |

### 環境変数レイヤー

環境変数レイヤーは、プレフィックスに一致する環境変数を設定として読み込みます：

```go
// APP_ プレフィックスの環境変数を読み込み
// APP_SERVER_HOST -> /server/host
// APP_DATABASE_USER -> /database/user
store.Add(
env.New("env", "APP_"),
jubako.WithPriority(jubako.PriorityEnv),
)
```

**注意**: 環境変数は常に文字列として読み込まれます。数値型のフィールドに環境変数で値を設定する場合は、YAML
レイヤーとの併用を検討してください。

詳しい使用例は [examples/env-override](examples/env-override/) を参照してください。

## 独自フォーマット・ソースの作成

Jubako は拡張可能なアーキテクチャを持っています。独自のフォーマットやソースを実装できます。

### Source インターフェース

Source は設定データの入出力を担当します（フォーマットに依存しない）：

```go
// source/source.go
type Source interface {
// Load はソースから設定データを読み込みます
Load(ctx context.Context) ([]byte, error)

// Save はデータをソースに書き込みます
// 保存をサポートしない場合は ErrSaveNotSupported を返します
Save(ctx context.Context, data []byte) error

// CanSave は保存をサポートするかを返します
CanSave() bool
}
```

**実装例（HTTP ソース）**:

```go
type HTTPSource struct {
url string
}

func NewHTTP(url string) *HTTPSource {
return &HTTPSource{url: url}
}

func (s *HTTPSource) Load(ctx context.Context) ([]byte, error) {
req, err := http.NewRequestWithContext(ctx, "GET", s.url, nil)
if err != nil {
return nil, err
}
resp, err := http.DefaultClient.Do(req)
if err != nil {
return nil, err
}
defer resp.Body.Close()
return io.ReadAll(resp.Body)
}

func (s *HTTPSource) Save(ctx context.Context, data []byte) error {
return source.ErrSaveNotSupported
}

func (s *HTTPSource) CanSave() bool {
return false
}

// 使用例
store.Add(
layer.New("remote", NewHTTP("https://config.example.com/app.yaml"), yaml.NewParser()),
jubako.WithPriority(jubako.PriorityDefaults),
)
```

### Parser インターフェース

Parser は生のバイト列を Document に変換します：

```go
// document/parser.go
type Parser interface {
// Parse はバイト列を Document に変換します
Parse(data []byte) (Document, error)

// Format はこのパーサーが扱うフォーマットを返します
Format() DocumentFormat

// CanMarshal はコメント保持付きでマーシャル可能かを返します
CanMarshal() bool
}
```

### Document インターフェース

Document は構造化された設定データへのアクセスを提供します：

```go
// document/document.go
type Document interface {
// Get は指定パスの値を取得します（JSON Pointer）
Get(path string) (any, bool)

// Set は指定パスに値を設定します
Set(path string, value any) error

// Delete は指定パスの値を削除します
Delete(path string) error

// Marshal はドキュメントをバイト列にシリアライズします
// コメントとフォーマットを可能な限り保持します
Marshal() ([]byte, error)

// Format はドキュメントのフォーマットを返します
Format() DocumentFormat

// MarshalTestData はテスト用にデータをバイト列に変換します
MarshalTestData(data map[string]any) ([]byte, error)
}
```

### フォーマット実装の2つのアプローチ

独自フォーマットを実装する際は、2つのアプローチがあります：

#### 1. mapdoc による簡易実装

コメント保持が不要な場合、`mapdoc` パッケージを使用して簡単にフォーマットを追加できます。
`map[string]any` をバックエンドとし、JSON Pointer によるパスアクセスや中間マップの自動作成など、
基本的な機能がすべて提供されます。

**JSON フォーマットの実装例（約 30 行）**:

```go
// format/json/document.go
package json

import (
	"encoding/json"
	"fmt"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/mapdoc"
)

// Document は map[string]any をバックエンドとする JSON ドキュメント
type Document = mapdoc.Document

// Parse は JSON データをドキュメントに変換します
func Parse(data []byte) (*Document, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return mapdoc.New(
		document.FormatJSON,
		mapdoc.WithData(root),
		mapdoc.WithMarshal(marshalJSON),
	), nil
}

func marshalJSON(data map[string]any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
```

#### 2. AST ベースのフルサポート実装

コメントやフォーマットを保持する必要がある場合、フォーマット固有の AST を直接操作する
Document 実装が必要です。

**YAML フォーマットの実装概要**:

```go
// format/yaml/document.go
package yaml

import (
	"gopkg.in/yaml.v3"
	"github.com/yacchi/jubako/document"
)

// Document は yaml.Node AST をバックエンドとする YAML ドキュメント
type Document struct {
	root *yaml.Node // AST を直接保持
}

// Get は yaml.Node を走査して値を取得
func (d *Document) Get(path string) (any, bool) {
	// AST からノードを検索し、値に変換
}

// Set は yaml.Node を走査・更新
func (d *Document) Set(path string, value any) error {
	// 既存ノードを更新、または新規ノードを作成
	// コメントは既存ノードに紐づいているため保持される
}

// Marshal は AST をそのままシリアライズ
func (d *Document) Marshal() ([]byte, error) {
	return yaml.Marshal(d.root) // コメント付きで出力
}
```

TOML や JSONC も同様に、各ライブラリの AST を操作することでコメント保持を実現します。

### Layer インターフェース

Layer は Source と Parser を組み合わせた設定ソースを表します。
通常は `layer.New()` で作成される `SourceParser` 実装を使用しますが、
環境変数レイヤーのように特殊な実装も可能です：

```go
// layer/layer.go
type Layer interface {
// Name はレイヤーの一意な識別子を返します
Name() Name

// Load は設定を読み込み Document を返します
Load(ctx context.Context) (Document, error)

// Document は読み込み済みの Document を返します
Document() Document

// Save は Document をソースに保存します
Save(ctx context.Context) error

// CanSave は保存をサポートするかを返します
CanSave() bool
}
```

既存の実装については以下のパッケージを参照してください：

- `source/bytes` - Byte slice source（read-only）
- `source/fs` - File system source
- `format/yaml` - YAML parser（AST-based, format preservation）
- `format/toml` - TOML parser（separate module, comment + format preservation）
- `format/jsonc` - JSONC parser（separate module, comment + format preservation）
- `format/json` - JSON parser（mapdoc-based, simple）
- `layer/env` - Environment variable layer

## 一般的な設定ライブラリとの比較

| 機能       | Jubako                 | 一般的なライブラリ |
|----------|------------------------|-----------|
| レイヤー管理   | レイヤーごとに保持              | マージ後は区別不可 |
| 値の出所追跡   | 対応                     | 非対応       |
| 書き込み     | レイヤーを指定して書き戻し可能        | 限定的       |
| フォーマット保持 | 対応（AST ベース、対応フォーマットのみ） | 非対応       |

## ライセンス

MIT License

## コントリビューション

コントリビューションは歓迎します！お気軽に Pull Request を送ってください。
