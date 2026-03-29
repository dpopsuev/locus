package = "testkit"
version = "0.1-1"
source = { url = "file://." }
dependencies = { "luasocket >= 3.0" }
build = { type = "builtin", modules = { entity = "src/entity.lua" } }
