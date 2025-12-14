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
    - [パスリマッピング (jubako タグ)](#パスリマッピング-jubako-タグ)
    - [カスタムデコーダー](#カスタムデコーダー)
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
	if err := store.Add(
		layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Add(
		layer.New("user", fs.New("~/.config/app/config.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityUser),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Add(
		layer.New("project", fs.New(".app.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityProject),
	); err != nil {
		log.Fatal(err)
	}

	if err := store.Add(
		env.New("env", "APP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	); err != nil {
		log.Fatal(err)
	}

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
- [path-remapping](examples/path-remapping/) - jubako タグによるパスリマッピング（絶対/相対パス、slice/map 対応）
- [custom-decoder](examples/custom-decoder/) - mapstructure を使ったカスタムデコーダー

## コアコンセプト

### レイヤー

各設定ソースは優先度を持つレイヤーとして表現されます。優先度の高いレイヤーが低いレイヤーの値を上書きします。

```go
package main

import "github.com/yacchi/jubako"

func main() {
	_ = jubako.PriorityDefaults // 0: 最低
	_ = jubako.PriorityUser     // 10
	_ = jubako.PriorityProject  // 20
	_ = jubako.PriorityEnv      // 30
	_ = jubako.PriorityFlags    // 40: 最高
}
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
package main

import "github.com/yacchi/jubako/jsonptr"

func main() {
	// ポインタを構築
	ptr1 := jsonptr.Build("server", "port")     // "/server/port"
	ptr2 := jsonptr.Build("servers", 0, "name") // "/servers/0/name"

	// ポインタを解析
	keys, _ := jsonptr.Parse("/server/port") // ["server", "port"]

	// 特殊文字の処理
	ptr3 := jsonptr.Build("feature.flags", "on/off") // "/feature.flags/on~1off"

	_ = ptr1
	_ = ptr2
	_ = ptr3
	_ = keys
}
```

**エスケープルール（RFC 6901）：**

- `~` は `~0` としてエンコード
- `/` は `~1` としてエンコード

### 設定構造体の定義

設定構造体を定義する際は、`json` タグが必須です。
マテリアライズ処理では内部的に `encoding/json` を使ってマージ済みのマップを構造体にデコードします。
必要に応じて `yaml` や `toml` などのフォーマット固有タグも付与してください。

```go
package main

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
```

**高度な機能：**

- [jubako タグ](#パスリマッピング-jubako-タグ)を使用してネストされた設定パスをフラットな構造体フィールドにリマップ
- [カスタムデコーダー](#カスタムデコーダー)（mapstructure 等）でより柔軟なデコードを実現

### パスリマッピング (jubako タグ)

`jubako` 構造体タグを使用すると、ネストされた設定パスをフラットな構造体フィールドにリマップできます。
設定ファイルは可読性のためにネスト構造を使用し、アプリケーションコードでは利便性のためにフラットな構造体を使用したい場合に便利です。

#### パスの種類

| 形式 | 例 | 説明 |
|-----|---|-----|
| 絶対パス | `/server/host` | ルートから解決（`/` で始まる） |
| 相対パス | `connection/host` | 現在のコンテキストから解決 |
| 相対パス（明示的） | `./connection/host` | 上記と同じ、明示的な構文 |

#### 基本的な使い方

```go
package main

// 設定ファイルの構造（可読性のためにネスト）:
//   server:
//     http:
//       read_timeout: 30
//       write_timeout: 60
//
// Go 構造体（利便性のためにフラット）:
type ServerConfig struct {
	Host         string `json:"host" jubako:"/server/host"`
	Port         int    `json:"port" jubako:"/server/port"`
	ReadTimeout  int    `json:"read_timeout" jubako:"/server/http/read_timeout"`
	WriteTimeout int    `json:"write_timeout" jubako:"/server/http/write_timeout"`
	// リマッピングから除外
	Internal     string `json:"internal" jubako:"-"`
}
```

#### スライス要素での相対パス

構造体要素がスライス内にある場合、相対パス（先頭に `/` なし）を使用して
各要素のコンテキストから解決します：

```go
package main

// 設定ファイルの構造:
//   defaults:
//     timeout: 30
//   nodes:
//     - connection:
//         host: node1.example.com
//         port: 5432
//     - connection:
//         host: node2.example.com
//         port: 5433

type Node struct {
	// 相対パス - 各スライス要素から解決
	Host string `json:"host" jubako:"connection/host"`
	Port int    `json:"port" jubako:"connection/port"`
	// 絶対パス - ルートから解決（全要素で共有）
	DefaultTimeout int `json:"default_timeout" jubako:"/defaults/timeout"`
}

type ClusterConfig struct {
	Nodes []Node `json:"nodes"`
}
```

#### マップ値での相対パス

マップ値でも同じパターンが使用できます：

```go
package main

// 設定ファイルの構造:
//   defaults:
//     retries: 3
//   services:
//     api:
//       settings:
//         endpoint: https://api.example.com
//     web:
//       settings:
//         endpoint: https://web.example.com

type ServiceConfig struct {
	// 相対パス - 各マップ値から解決
	Endpoint string `json:"endpoint" jubako:"settings/endpoint"`
	// 絶対パス - ルートから解決
	DefaultRetries int `json:"default_retries" jubako:"/defaults/retries"`
}

type Config struct {
	Services map[string]ServiceConfig `json:"services"`
}
```

#### マッピングテーブルの確認

デバッグ用にマッピングテーブルを確認できます：

```go
store := jubako.New[ClusterConfig]()

// jubako マッピングがあるか確認
if store.HasMappings() {
	fmt.Println(store.MappingTable())
}

// 出力:
// nodes[]: (slice element)
//   host <- ./connection/host (relative)
//   port <- ./connection/port (relative)
//   default_timeout <- /defaults/timeout
```

完全な動作例は [examples/path-remapping](examples/path-remapping/) を参照してください。

### カスタムデコーダー

デフォルトでは、Jubako は `encoding/json` を使用してマージ済みの `map[string]any` を設定構造体に変換します。
`WithDecoder` オプションを使用してこのデコーダーを置き換えることができます：

```go
store := jubako.New[Config](jubako.WithDecoder(myCustomDecoder))
```

デコーダーは `decoder.Func` 型に一致する必要があります：

```go
type Func func(data map[string]any, target any) error
```

**カスタムデコーダーを使用する場面：**

- カスタム構造体タグを使用（例：`json` の代わりに `mapstructure`）
- 弱い型変換を有効化（例：文字列 `"8080"` を int `8080` に変換）
- 埋め込み構造体や残りフィールドのキャプチャを処理
- 既存のデコードパイプラインとの統合

完全な使用例は [examples/custom-decoder](examples/custom-decoder/) を参照してください（[mapstructure](https://github.com/mitchellh/mapstructure) を使用）。

## API リファレンス

### Store[T]

Store は設定管理の中心となる型です。

#### 作成と設定

```go
package main

import "github.com/yacchi/jubako"

type AppConfig struct{}

func main() {
	// 新しいストアを作成
	store := jubako.New[AppConfig]()

	// 自動優先度のステップを指定（デフォルト: 10）
	storeWithStep := jubako.New[AppConfig](jubako.WithPriorityStep(100))

	_ = store
	_ = storeWithStep
}
```

#### レイヤーの追加

```go
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/bytes"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct{}

const (
	defaultsYAML = ""
	baseYAML     = ""
	overrideYAML = ""
)

func main() {
	store := jubako.New[AppConfig]()

	// 優先度を指定してレイヤーを追加
	err := store.Add(
		layer.New("defaults", bytes.FromString(defaultsYAML), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
	)

	// 読み取り専用として追加（SetTo による変更を禁止）
	err = store.Add(
		layer.New("system", fs.New("/etc/app/config.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityDefaults),
		jubako.WithReadOnly(),
	)

	// 優先度を省略すると追加順に自動割り当て（0, 10, 20, ...）
	err = store.Add(layer.New("base", bytes.FromString(baseYAML), yaml.NewParser()))
	err = store.Add(layer.New("override", bytes.FromString(overrideYAML), yaml.NewParser()))

	_ = err
}
```

#### 読み込みとアクセス

```go
package main

import (
	"context"
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct {
	Server struct {
		Port int `json:"port"`
	} `json:"server"`
}

func main() {
	ctx := context.Background()
	store := jubako.New[AppConfig]()

	// 全レイヤーを読み込み
	err := store.Load(ctx)

	// 設定をリロード
	err = store.Reload(ctx)

	// マージ済み設定を取得
	config := store.Get()
	fmt.Println(config.Server.Port)

	_ = err
}
```

#### 変更通知

```go
package main

import (
	"log"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// 設定変更をサブスクライブ
	unsubscribe := store.Subscribe(func(cfg AppConfig) {
		log.Printf("Config changed: %+v", cfg)
	})
	defer unsubscribe()
}
```

#### 値の変更と保存

```go
package main

import (
	"context"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	ctx := context.Background()
	store := jubako.New[AppConfig]()

	// 特定レイヤーの値を変更（メモリ上）
	err := store.SetTo("user", "/server/port", 9000)

	// 変更があるか確認
	if store.IsDirty() {
		// 変更された全レイヤーを保存
		err = store.Save(ctx)

		// または特定レイヤーのみ保存
		err = store.SaveLayer(ctx, "user")
	}

	_ = err
}
```

### オリジン追跡

各設定値がどのレイヤーから来たかを追跡できます。

#### GetAt - 単一値の取得

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	rv := store.GetAt("/server/port")
	if rv.Exists {
		fmt.Printf("port=%v (from layer %s)\n", rv.Value, rv.Layer.Name())
	}
}
```

#### GetAllAt - 全レイヤーの値を取得

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	values := store.GetAllAt("/server/port")
	for _, rv := range values {
		fmt.Printf("port=%v (from layer %s, priority %d)\n",
			rv.Value, rv.Layer.Name(), rv.Layer.Priority())
	}

	// 最も優先度の高い値を取得
	effective := values.Effective()
	fmt.Printf("effective: %v\n", effective.Value)
}
```

#### Walk - 全設定値を走査

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// 各パスの解決済み値を取得
	store.Walk(func(ctx jubako.WalkContext) bool {
		rv := ctx.Value()
		fmt.Printf("%s = %v (from %s)\n", ctx.Path, rv.Value, rv.Layer.Name())
		return true // 継続
	})

	// 各パスの全レイヤー値を取得（オーバーライドチェーンの分析）
	store.Walk(func(ctx jubako.WalkContext) bool {
		allValues := ctx.AllValues()
		if allValues.Len() > 1 {
			fmt.Printf("%s has values from %d layers:\n", ctx.Path, allValues.Len())
			for _, rv := range allValues {
				fmt.Printf("  - %s: %v\n", rv.Layer.Name(), rv.Value)
			}
		}
		return true
	})
}
```

詳しい使用例は [examples/origin-tracking](examples/origin-tracking/) を参照してください。

### レイヤー情報

```go
package main

import (
	"fmt"

	"github.com/yacchi/jubako"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

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
| TOML   | `format/toml`  | コメント/フォーマットを保持したまま編集・保存可能              |
| JSONC  | `format/jsonc` | コメント/フォーマットを保持したまま編集・保存可能              |

```go
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/yaml"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// YAML（コメント保持）
	_ = store.Add(
		layer.New("user", fs.New("~/.config/app.yaml"), yaml.NewParser()),
		jubako.WithPriority(jubako.PriorityUser),
	)
}

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
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/format/json"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/source/fs"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// JSON（コメント非保持）
	_ = store.Add(
		layer.New("config", fs.New("config.json"), json.NewParser()),
		jubako.WithPriority(jubako.PriorityProject),
	)
}
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
package main

import (
	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/env"
)

type AppConfig struct{}

func main() {
	store := jubako.New[AppConfig]()

	// APP_ プレフィックスの環境変数を読み込み
	// APP_SERVER_HOST -> /server/host
	// APP_DATABASE_USER -> /database/user
	_ = store.Add(
		env.New("env", "APP_"),
		jubako.WithPriority(jubako.PriorityEnv),
	)
}
```

**注意**: 環境変数は常に文字列として読み込まれます。数値型のフィールドに環境変数で値を設定する場合は、YAML
レイヤーとの併用を検討してください。

詳しい使用例は [examples/env-override](examples/env-override/) を参照してください。

## 独自フォーマット・ソースの作成

Jubako は拡張可能なアーキテクチャを持っています。独自のフォーマットやソースを実装できます。

### Source インターフェース

Source は設定データの入出力を担当します（フォーマットに依存しない）：

```go
package source

import "context"

// source/source.go
type Source interface {
	// Load はソースから設定データを読み込みます。
	Load(ctx context.Context) ([]byte, error)

	// Save はデータをソースに書き込みます。
	// 保存をサポートしない場合は ErrSaveNotSupported を返します。
	Save(ctx context.Context, data []byte) error

	// CanSave は保存をサポートするかを返します。
	CanSave() bool
}
```

**実装例（HTTP ソース）**:

```go
package main

import (
	"context"
	"io"
	"net/http"

	"github.com/yacchi/jubako/source"
)

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

// 使用例:
//
//  store.Add(
//      layer.New("remote", NewHTTP("https://config.example.com/app.yaml"), yaml.NewParser()),
//      jubako.WithPriority(jubako.PriorityDefaults),
//  )
```

### Parser インターフェース

Parser は生のバイト列を Document に変換します：

```go
package document

type Document interface{}
type DocumentFormat string

// document/parser.go
type Parser interface {
	// Parse はバイト列を Document に変換します。
	Parse(data []byte) (Document, error)

	// Format はこのパーサーが扱うフォーマットを返します。
	Format() DocumentFormat

	// CanMarshal はコメント保持付きでマーシャル可能かを返します。
	CanMarshal() bool
}
```

### Document インターフェース

Document は構造化された設定データへのアクセスを提供します：

```go
package document

type DocumentFormat string

// document/document.go
type Document interface {
	// Get は指定パスの値を取得します（JSON Pointer）。
	Get(path string) (any, bool)

	// Set は指定パスに値を設定します。
	Set(path string, value any) error

	// Delete は指定パスの値を削除します。
	Delete(path string) error

	// Marshal はドキュメントをバイト列にシリアライズします。
	// コメントとフォーマットを可能な限り保持します。
	Marshal() ([]byte, error)

	// Format はドキュメントのフォーマットを返します。
	Format() DocumentFormat

	// MarshalTestData はテスト用にデータをバイト列に変換します。
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
	"fmt"

	"github.com/yacchi/jubako/document"
	"gopkg.in/yaml.v3"
)

// Document は yaml.Node AST をバックエンドとする YAML ドキュメント
type Document struct {
	root *yaml.Node // AST を直接保持
}

var _ document.Document = (*Document)(nil)

// Get は yaml.Node を走査して値を取得します。
func (d *Document) Get(path string) (any, bool) { return nil, false }

// Set は yaml.Node を走査・更新します。
func (d *Document) Set(path string, value any) error { return nil }

// Delete は指定パスの値を削除します。
func (d *Document) Delete(path string) error { return nil }

// Marshal は AST をそのままシリアライズ
func (d *Document) Marshal() ([]byte, error) {
	return yaml.Marshal(d.root) // コメント付きで出力
}

// Format はドキュメントのフォーマットを返します。
func (d *Document) Format() document.DocumentFormat { return document.FormatYAML }

// MarshalTestData はテスト用にデータを YAML に変換します。
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	if data == nil {
		data = map[string]any{}
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test data: %w", err)
	}
	return b, nil
}
```

TOML や JSONC も同様に、各ライブラリの AST を操作することでコメント保持を実現します。

### Layer インターフェース

Layer は Source と Parser を組み合わせた設定ソースを表します。
通常は `layer.New()` で作成される `SourceParser` 実装を使用しますが、
環境変数レイヤーのように特殊な実装も可能です：

```go
package layer

import (
	"context"

	"github.com/yacchi/jubako/document"
)

type Name string

// layer/layer.go
type Layer interface {
	// Name はレイヤーの一意な識別子を返します。
	Name() Name

	// Load は設定を読み込み Document を返します。
	Load(ctx context.Context) (document.Document, error)

	// Document は読み込み済みの Document を返します。
	Document() document.Document

	// Save は Document をソースに保存します。
	Save(ctx context.Context) error

	// CanSave は保存をサポートするかを返します。
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
