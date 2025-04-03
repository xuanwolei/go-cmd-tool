# gen-interface

一个用于自动生成 Go 接口定义的命令行工具。从结构体实现中提取方法签名，并生成对应的接口定义文件。

## 特性

- 自动从结构体实现中生成接口定义
- 支持方法注释的保留和迁移
- 智能导入包管理（仅保留被使用的导入）
- 支持生成注册函数（用于依赖注入）
- 支持生成 mockgen 指令（用于单元测试）
- 支持自定义接口命名前缀
- 支持文件和目录的排除规则
- 支持文件和目录的包含规则
- 支持自定义结构体名称匹配模式
- 智能处理方法参数和返回值
- 保持原有代码格式和注释风格

## 安装

```bash
go install github.com/xuanwolei/go-cmd-tool/cmd/gen-interface@latest
```

## 使用方法
基本用法：

```bash
gen-interface -s ./src -d ./interfaces
```

### 完整参数说明：

```bash
gen-interface [flags]

Flags:
  -s, --src string          源目录路径（必需）
  -d, --dst string          目标目录路径（必需）
  -i, --include string      包含的文件或目录，多个用逗号分隔
  -e, --exclude string      排除的文件或目录，多个用逗号分隔，优先级高于include
  -p, --stPattern string    结构体名称匹配的正则表达式（默认 "^.+Dao$"）
  -f, --prefix string       接口前缀（默认 "I"）
  -r, --generateRegister    是否生成实体变量和注册函数（默认 false）
  -m, --generateMock        是否生成 mockgen 指令（默认 false）
  -k, --mockPath string     mock 文件的生成路径（默认 "../mocks"）
```

## 示例
假设有以下结构体实现：

```go
// user_dao.go
package dao

type UserDao struct {}

// CreateUser 创建用户
func (d *UserDao) CreateUser(name string) error {
    // 实现...
}

// GetUser 获取用户
func (d *UserDao) GetUser(id int64) (*User, error) {
    // 实现...
}
```

运行命令：

```bash
gen-interface -s ./dao -d ./interfaces
```

将生成：

```go
// interfaces/user_dao.go
package interfaces

// IUserDao 是 UserDao 的接口定义
type IUserDao interface {
    // CreateUser 创建用户
    CreateUser(name string) error
    // GetUser 获取用户
    GetUser(id int64) (*User, error)
}
```

### 生成注册函数
使用 -r 标志可以生成注册函数：

```bash
gen-interface -s ./dao -d ./interfaces -r
```

将额外生成：

```go
var (
    localIUserDao IUserDao
)

func UserDao() IUserDao {
    if localIUserDao == nil {
        panic("implement not found for interface IUserDao, forgot register?")
    }
    return localIUserDao
}

func RegisterUserDao(i IUserDao) {
    localIUserDao = i
}
```

### 生成 Mock
使用 -m 标志可以生成 mockgen 指令：

```bash
gen-interface -s ./dao -d ./interfaces -m -k../mocks
```

将在接口定义前添加：

```go
//go:generate mockgen -source=user_dao.go -destination=../mocks/user_dao.go -package=mocks
```

### 使用包含和排除规则
只处理特定文件：

```bash
gen-interface -s ./dao -d ./interfaces -i "user*.go,order*.go"

```

排除特定文件：

```bash
gen-interface -s ./dao -d ./interfaces -e "internal/*,test/*"

```

### 自定义结构体匹配
匹配所有 Service 结尾的结构体：

```bash
gen-interface -s ./service -d ./interfaces -p "^.+Service$"
```

## 注意事项
1. 只会处理公开的方法（首字母大写）
2. 排除规则优先级高于包含规则

