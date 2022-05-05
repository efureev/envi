# Envi is a package to manage `.env` files

[![Go package](https://github.com/efureev/envi/actions/workflows/go.yml/badge.svg)](https://github.com/efureev/envi/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/envi)](https://goreportcard.com/report/github.com/efureev/envi)

## Installation

As a library

```shell
go get github.com/efureev/envi
```

## Description

`Envi` allows you to:

- parse, load and save `.env`-files
- load multiple `.env`-files in a row
- fully manage your data in `Env`-structure like as:
    - division by blocks and rows
    - commented items
    - set item's comments (blocks and rows)
    - add or remove items (blocks and rows)
    - sorting items

## Usage

For example, `.env` file to parse:

```dotenv
###   ---[ Application section ]---   ###
# Application name
APP_NAME="App name"
APP_DEBUG=false

# Default dev.host
# APP_URL=http://dev.example.com
APP_URL=https://example.com

###   ---[ NGINX cache section ]---   ###
# Nginx cache path
CACHE_NGINX_PATH=./storage/cache
# Enable caching a page
CACHE_NGINX_ENABLED=false

TEST=false

#APP_TRACE_LOAD=true
DEBUGBAR_ENABLED=false

#HYPE=false
```

Here we see:

- 3 `Block`s:
    - `APP`: Has Comment. Contains rows:
        - `APP_NAME`: Has Comment
        - `APP_DEBUG`
        - `APP_URL`: Has `shadow`: `http://dev.example.com`
        - `APP_TRACE_LOAD`: Commented row
    - `CACHE`: Has Comment. Contains rows:
        - `CACHE_NGINX_PATH`: Has a Comment
        - `CACHE_NGINX_ENABLED`: Has a Comment
    - `DEBUGBAR`:
        - `DEBUGBAR_ENABLED`
- 2 `Row`s:
    - `HYPE`: Commented row
    - `TEST`: Uncommented row

### Blocks

The Block is defined by first occurrence `_` in row. You may set up minimum rows count to form a `Block`.
By default, it's `1`.

Block may have a comment.

```go
block := envi.NewBlock(prefix string).SetComment(`Application section`)
```

You may change indent after `Block`:

```go
envi.SetIndent(2)
```

You may change a template of a block comment:

```go
envi.SetCommentTemplate(`# <-- `, ` -->`)
// or
envi.SetCommentTemplateByDefault()
```

You may set row count to form a block from them:

```go
envi.GroupRowsGreaterThen(1)
```

### Rows

A `Row` may have a comment.

```go
row := envi.NewRow(`app-section`, 'section 345').SetComment(`Section for the unit '345'`)
```

A `Row` may be commented.

```go
row.Commented()
```

A `Row` may be a part of `Block` or not.

```go
env := envi.Env{}
block := envi.NewBlock(`app`).AddRow(`session`, `test`)
env.Add(block, NewRow(`app-hash`, `sha-256`))

env.Save(`.env.local`)
```

### How to load `.env` files

```go
env, err := envi.Load(`stubs/.env`) // for single file

// to override multi files
env, err := envi.Load(`stubs/.env`, `stubs/.env.development`, `stubs/.env.development.local`)

/// ...

env.Save(`.env.finish`)
```

### How to manipulate data after parsing `.env` files

```go
env, err := envi.Load(`stubs/.env`)

// Total count rows (including in blocks)
env.Count()

// To receive a row by key
row := env.Get(`APP_URL`)

// To receive a block by prefix
block := env.GetBlock(`app`)

// To remove a row from data-structure
env.RemoveRow(`app-url`)

// To remove a Block from data-structure
env.RemoveGroup(`app`)

// To add a row or a block
env.Add(NewRow(`key`, `val`))
env.Add(NewBlock(`prefix`))

// To merge
env, err := envi.Load(`stubs/.env`)
env2, err := envi.Load(`stubs/.env.local`)
env.Merge(env2)

// To merge items
env.MergeItems(block1, row1, row3, block2)

// To set system environment
override := true // override existing envs
env.SetEnv(override)
```

In blocks

```go

// To create a new Block
block := envi.NewBlock(`APP`)

// To Receive a Block from `Envi` 
block := env.GetBlock(`APP`)

// Rows count in the block
block.Count()

// To receive a row from a block by key without block's prefix. Not: `app-hash`!
r = block.GetRow(`hash`)

// To receive a row from a block by key with block's prefix. Allow: `app-hash`
r = block.GetPrefixedRow(`hash`)

// To add rows into a block
block.AddRows(rows2, rows1, envi.NewRow(`section`, `1`), ...)

// To add fully prefixed rows
block.AddPrefixedRows(envi.NewRow(`APP-section`, `1`), ...)

// To set block's comment
block.SetComment(`New Comment`)

// To merge with other block
block.MergeBlock(block2)

// To merge with row
block.MergeRow(row)

// To find out if this row exists
block.HasRow(row)

// To remove a row from the Block by key
block.RemoveRow(`section`)

// To remove a row from the Block by fully key
block.RemovePrefixedRow(`app-section`)
```

In rows

```go

// To create a new row
row := envi.NewRow(`app-section`, `two`)

// To set a comment
row.SetComment(`new section`)

// To comment row
row.Commented()

// To merge with other row
row.Merge()

// To get full key
row.GetFullKey()

```
