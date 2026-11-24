package main

// langWords holds the keyword and native-type sets for one language.
// foldCase is for languages whose keywords are case-insensitive
// (SQL): the word is upper-cased before lookup, so lowercase
// `select` classifies the same as `SELECT`.
type langWords struct {
	keywords map[string]bool
	types    map[string]bool
	foldCase bool
}

// keywordTable uses extractor language names. Words absent from both sets
// stay identifiers, including all words in unknown languages.
var keywordTable = map[string]langWords{
	"go": {
		keywords: map[string]bool{
			"break": true, "case": true, "chan": true, "const": true, "continue": true,
			"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
			"func": true, "go": true, "goto": true, "if": true, "import": true,
			"interface": true, "map": true, "package": true, "range": true, "return": true,
			"select": true, "struct": true, "switch": true, "type": true, "var": true,
		},
		types: map[string]bool{
			"any": true, "bool": true, "byte": true, "complex64": true, "complex128": true,
			"error": true, "float32": true, "float64": true, "int": true, "int8": true,
			"int16": true, "int32": true, "int64": true, "rune": true, "string": true,
			"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
			"uintptr": true,
		},
	},
	"powershell": {
		keywords: map[string]bool{
			"begin": true, "break": true, "catch": true, "class": true, "continue": true,
			"data": true, "do": true, "dynamicparam": true, "else": true, "elseif": true,
			"end": true, "enum": true, "exit": true, "filter": true, "finally": true,
			"for": true, "foreach": true, "from": true, "function": true, "if": true,
			"in": true, "param": true, "process": true, "return": true, "switch": true,
			"throw": true, "trap": true, "try": true, "until": true, "using": true,
			"while": true,
		},
		types: map[string]bool{
			"array": true, "bool": true, "byte": true, "char": true, "datetime": true,
			"decimal": true, "double": true, "float": true, "hashtable": true, "int": true,
			"long": true, "object": true, "scriptblock": true, "string": true, "void": true,
		},
	},
	"javascript": {
		keywords: map[string]bool{
			"async": true, "await": true, "break": true, "case": true, "catch": true,
			"class": true, "const": true, "continue": true, "debugger": true, "default": true,
			"delete": true, "do": true, "else": true, "export": true, "extends": true,
			"false": true, "finally": true, "for": true, "function": true, "if": true,
			"import": true, "in": true, "instanceof": true, "let": true, "new": true,
			"null": true, "return": true, "static": true, "super": true, "switch": true,
			"this": true, "throw": true, "true": true, "try": true, "typeof": true,
			"var": true, "void": true, "while": true, "with": true, "yield": true,
		},
		types: map[string]bool{
			"Array": true, "BigInt": true, "Boolean": true, "Function": true, "Map": true,
			"Number": true, "Object": true, "Promise": true, "Set": true, "String": true,
			"Symbol": true,
		},
	},
	"typescript": {
		keywords: map[string]bool{
			"abstract": true, "async": true, "await": true, "break": true, "case": true,
			"catch": true, "class": true, "const": true, "continue": true, "declare": true,
			"default": true, "delete": true, "do": true, "else": true, "enum": true,
			"export": true, "extends": true, "false": true, "finally": true, "for": true,
			"function": true, "if": true, "implements": true, "import": true, "in": true,
			"instanceof": true, "interface": true, "let": true, "new": true, "null": true,
			"private": true, "protected": true, "public": true, "readonly": true, "return": true,
			"static": true, "super": true, "switch": true, "this": true, "throw": true,
			"true": true, "try": true, "type": true, "typeof": true, "var": true,
			"while": true, "yield": true,
		},
		types: map[string]bool{
			"any": true, "bigint": true, "boolean": true, "never": true, "number": true,
			"object": true, "string": true, "symbol": true, "undefined": true, "unknown": true,
			"void": true,
		},
	},
	"rust": {
		keywords: map[string]bool{
			"as": true, "async": true, "await": true, "break": true, "const": true,
			"continue": true, "crate": true, "dyn": true, "else": true, "enum": true,
			"extern": true, "false": true, "fn": true, "for": true, "if": true,
			"impl": true, "in": true, "let": true, "loop": true, "match": true,
			"mod": true, "move": true, "mut": true, "pub": true, "ref": true,
			"return": true, "self": true, "Self": true, "static": true, "struct": true,
			"super": true, "trait": true, "true": true, "type": true, "unsafe": true,
			"use": true, "where": true, "while": true,
		},
		types: map[string]bool{
			"bool": true, "char": true, "f32": true, "f64": true, "i8": true,
			"i16": true, "i32": true, "i64": true, "i128": true, "isize": true,
			"str": true, "u8": true, "u16": true, "u32": true, "u64": true,
			"u128": true, "usize": true,
		},
	},
	"c": {
		keywords: map[string]bool{
			"auto": true, "break": true, "case": true, "const": true, "continue": true,
			"default": true, "do": true, "else": true, "enum": true, "extern": true,
			"for": true, "goto": true, "if": true, "inline": true, "register": true,
			"restrict": true, "return": true, "sizeof": true, "static": true, "struct": true,
			"switch": true, "typedef": true, "union": true, "volatile": true, "while": true,
			"_Alignas": true, "_Alignof": true, "_Atomic": true, "_Generic": true, "_Static_assert": true,
			"_Thread_local": true,
		},
		types: map[string]bool{
			"_Bool": true, "_Complex": true, "bool": true, "char": true, "double": true,
			"float": true, "int": true, "long": true, "short": true, "signed": true,
			"unsigned": true, "void": true,
		},
	},
	"cpp": {
		keywords: map[string]bool{
			"alignas": true, "alignof": true, "auto": true, "break": true, "case": true,
			"catch": true, "class": true, "const": true, "constexpr": true, "continue": true,
			"decltype": true, "default": true, "delete": true, "do": true, "else": true,
			"enum": true, "explicit": true, "extern": true, "false": true, "for": true,
			"friend": true, "if": true, "inline": true, "namespace": true, "new": true,
			"noexcept": true, "nullptr": true, "operator": true, "private": true, "protected": true,
			"public": true, "return": true, "sizeof": true, "static": true, "struct": true,
			"switch": true, "template": true, "this": true, "throw": true, "true": true,
			"try": true, "typedef": true, "typename": true, "union": true, "using": true,
			"virtual": true, "volatile": true, "while": true,
		},
		types: map[string]bool{
			"bool": true, "char": true, "char8_t": true, "char16_t": true, "char32_t": true,
			"double": true, "float": true, "int": true, "long": true, "short": true,
			"signed": true, "unsigned": true, "void": true, "wchar_t": true,
		},
	},
	"csharp": {
		keywords: map[string]bool{
			"abstract": true, "as": true, "base": true, "break": true, "case": true,
			"catch": true, "checked": true, "class": true, "const": true, "continue": true,
			"default": true, "delegate": true, "do": true, "else": true, "enum": true,
			"event": true, "false": true, "finally": true, "for": true, "foreach": true,
			"if": true, "in": true, "interface": true, "is": true, "lock": true,
			"namespace": true, "new": true, "null": true, "out": true, "override": true,
			"private": true, "protected": true, "public": true, "readonly": true, "ref": true,
			"return": true, "sealed": true, "static": true, "struct": true, "switch": true,
			"this": true, "throw": true, "true": true, "try": true, "unchecked": true,
			"unsafe": true, "using": true, "virtual": true, "while": true,
		},
		types: map[string]bool{
			"bool": true, "byte": true, "char": true, "decimal": true, "double": true,
			"float": true, "int": true, "long": true, "object": true, "sbyte": true,
			"short": true, "string": true, "uint": true, "ulong": true, "ushort": true,
			"void": true,
		},
	},
	"java": {
		keywords: map[string]bool{
			"abstract": true, "assert": true, "break": true, "case": true, "catch": true,
			"class": true, "const": true, "continue": true, "default": true, "do": true,
			"else": true, "enum": true, "extends": true, "false": true, "final": true,
			"finally": true, "for": true, "goto": true, "if": true, "implements": true,
			"import": true, "instanceof": true, "interface": true, "native": true, "new": true,
			"null": true, "package": true, "private": true, "protected": true, "public": true,
			"return": true, "static": true, "super": true, "switch": true, "synchronized": true,
			"this": true, "throw": true, "throws": true, "transient": true, "true": true,
			"try": true, "volatile": true, "while": true,
		},
		types: map[string]bool{
			"boolean": true, "byte": true, "char": true, "double": true, "float": true,
			"int": true, "long": true, "short": true, "void": true,
		},
	},
	"kotlin": {
		keywords: map[string]bool{
			"as": true, "break": true, "class": true, "continue": true, "do": true,
			"else": true, "false": true, "for": true, "fun": true, "if": true,
			"in": true, "interface": true, "is": true, "null": true, "object": true,
			"package": true, "return": true, "super": true, "this": true, "throw": true,
			"true": true, "try": true, "typealias": true, "typeof": true, "val": true,
			"var": true, "when": true, "while": true,
		},
		types: map[string]bool{
			"Any": true, "Boolean": true, "Byte": true, "Char": true, "Double": true,
			"Float": true, "Int": true, "Long": true, "Nothing": true, "Short": true,
			"String": true, "UByte": true, "UInt": true, "ULong": true, "UShort": true,
			"Unit": true,
		},
	},
	"swift": {
		keywords: map[string]bool{
			"as": true, "associatedtype": true, "break": true, "case": true, "catch": true,
			"class": true, "continue": true, "default": true, "defer": true, "deinit": true,
			"do": true, "else": true, "enum": true, "extension": true, "fallthrough": true,
			"false": true, "fileprivate": true, "for": true, "func": true, "guard": true,
			"if": true, "import": true, "in": true, "init": true, "inout": true,
			"internal": true, "is": true, "let": true, "nil": true, "operator": true,
			"private": true, "protocol": true, "public": true, "repeat": true, "rethrows": true,
			"return": true, "self": true, "Self": true, "static": true, "struct": true,
			"subscript": true, "super": true, "switch": true, "throw": true, "throws": true,
			"true": true, "try": true, "typealias": true, "var": true, "where": true,
			"while": true,
		},
		types: map[string]bool{
			"Any": true, "Array": true, "Bool": true, "Character": true, "Dictionary": true,
			"Double": true, "Float": true, "Int": true, "Int8": true, "Int16": true,
			"Int32": true, "Int64": true, "Never": true, "String": true, "UInt": true,
			"Void": true,
		},
	},
	"scala": {
		keywords: map[string]bool{
			"abstract": true, "case": true, "catch": true, "class": true, "def": true,
			"do": true, "else": true, "extends": true, "false": true, "final": true,
			"finally": true, "for": true, "forSome": true, "if": true, "implicit": true,
			"import": true, "lazy": true, "match": true, "new": true, "null": true,
			"object": true, "override": true, "package": true, "private": true, "protected": true,
			"return": true, "sealed": true, "super": true, "this": true, "throw": true,
			"trait": true, "true": true, "try": true, "type": true, "val": true,
			"var": true, "while": true, "with": true, "yield": true,
		},
		types: map[string]bool{
			"Any": true, "AnyRef": true, "AnyVal": true, "Boolean": true, "Byte": true,
			"Char": true, "Double": true, "Float": true, "Int": true, "Long": true,
			"Nothing": true, "Null": true, "Short": true, "String": true, "Unit": true,
		},
	},
	"dart": {
		keywords: map[string]bool{
			"abstract": true, "as": true, "assert": true, "async": true, "await": true,
			"break": true, "case": true, "catch": true, "class": true, "const": true,
			"continue": true, "covariant": true, "default": true, "do": true, "else": true,
			"enum": true, "export": true, "extends": true, "extension": true, "external": true,
			"factory": true, "false": true, "final": true, "finally": true, "for": true,
			"if": true, "implements": true, "import": true, "in": true, "is": true,
			"late": true, "mixin": true, "new": true, "null": true, "rethrow": true,
			"return": true, "static": true, "super": true, "switch": true, "this": true,
			"throw": true, "true": true, "try": true, "typedef": true, "var": true,
			"while": true, "with": true, "yield": true,
		},
		types: map[string]bool{
			"bool": true, "double": true, "dynamic": true, "int": true, "List": true,
			"Map": true, "Never": true, "num": true, "Object": true, "Set": true,
			"String": true, "void": true,
		},
	},
	"groovy": {
		keywords: map[string]bool{
			"abstract": true, "as": true, "assert": true, "break": true, "case": true,
			"catch": true, "class": true, "const": true, "continue": true, "def": true,
			"default": true, "do": true, "else": true, "enum": true, "extends": true,
			"false": true, "final": true, "finally": true, "for": true, "if": true,
			"implements": true, "import": true, "in": true, "instanceof": true, "interface": true,
			"new": true, "null": true, "package": true, "private": true, "protected": true,
			"public": true, "return": true, "static": true, "super": true, "switch": true,
			"synchronized": true, "this": true, "throw": true, "throws": true, "trait": true,
			"true": true, "try": true, "while": true,
		},
		types: map[string]bool{
			"boolean": true, "byte": true, "char": true, "double": true, "float": true,
			"int": true, "long": true, "short": true, "void": true,
		},
	},
	"zig": {
		keywords: map[string]bool{
			"align": true, "allowzero": true, "and": true, "anyframe": true, "asm": true,
			"break": true, "catch": true, "comptime": true, "const": true, "continue": true,
			"defer": true, "else": true, "enum": true, "errdefer": true, "export": true,
			"extern": true, "fn": true, "for": true, "if": true, "inline": true,
			"linksection": true, "noalias": true, "noinline": true, "nosuspend": true, "opaque": true,
			"or": true, "orelse": true, "packed": true, "pub": true, "resume": true,
			"return": true, "struct": true, "suspend": true, "switch": true, "test": true,
			"threadlocal": true, "try": true, "union": true, "unreachable": true, "usingnamespace": true,
			"var": true, "volatile": true, "while": true,
		},
		types: map[string]bool{
			"anyerror": true, "anyopaque": true, "bool": true, "comptime_float": true, "comptime_int": true,
			"f16": true, "f32": true, "f64": true, "f128": true, "i32": true,
			"isize": true, "noreturn": true, "u8": true, "u32": true, "usize": true,
			"void": true,
		},
	},
	"css":  {},
	"json": {},
	"php": {
		keywords: map[string]bool{
			"abstract": true, "as": true, "break": true, "case": true, "catch": true,
			"class": true, "clone": true, "const": true, "continue": true, "declare": true,
			"default": true, "do": true, "echo": true, "else": true, "elseif": true,
			"empty": true, "enum": true, "extends": true, "final": true, "finally": true,
			"fn": true, "for": true, "foreach": true, "function": true, "global": true,
			"goto": true, "if": true, "implements": true, "include": true, "include_once": true,
			"instanceof": true, "interface": true, "isset": true, "match": true, "namespace": true,
			"new": true, "private": true, "protected": true, "public": true, "require": true,
			"require_once": true, "return": true, "static": true, "switch": true, "throw": true,
			"trait": true, "try": true, "unset": true, "use": true, "while": true,
			"yield": true,
		},
		types: map[string]bool{
			"array": true, "bool": true, "callable": true, "false": true, "float": true,
			"int": true, "iterable": true, "mixed": true, "never": true, "null": true,
			"object": true, "string": true, "true": true, "void": true,
		},
	},
	"python": {
		keywords: map[string]bool{
			"and": true, "as": true, "assert": true, "async": true, "await": true,
			"break": true, "class": true, "continue": true, "def": true, "del": true,
			"elif": true, "else": true, "except": true, "False": true, "finally": true,
			"for": true, "from": true, "global": true, "if": true, "import": true,
			"in": true, "is": true, "lambda": true, "None": true, "nonlocal": true,
			"not": true, "or": true, "pass": true, "raise": true, "return": true,
			"True": true, "try": true, "while": true, "with": true, "yield": true,
		},
		types: map[string]bool{
			"bool": true, "bytearray": true, "bytes": true, "complex": true, "dict": true,
			"float": true, "frozenset": true, "int": true, "list": true, "memoryview": true,
			"range": true, "set": true, "str": true, "tuple": true,
		},
	},
	"ruby": {
		keywords: map[string]bool{
			"alias": true, "and": true, "begin": true, "break": true, "case": true,
			"class": true, "def": true, "do": true, "else": true,
			"elsif": true, "end": true, "ensure": true, "false": true, "for": true,
			"if": true, "in": true, "module": true, "next": true, "nil": true,
			"not": true, "or": true, "redo": true, "rescue": true, "retry": true,
			"return": true, "self": true, "super": true, "then": true, "true": true,
			"undef": true, "unless": true, "until": true, "when": true, "while": true,
			"yield": true,
		},
		types: map[string]bool{
			"Array": true, "Complex": true, "FalseClass": true, "Float": true, "Hash": true,
			"Integer": true, "NilClass": true, "Numeric": true, "Rational": true, "Regexp": true,
			"String": true, "Symbol": true, "TrueClass": true,
		},
	},
	"perl": {
		keywords: map[string]bool{
			"continue": true, "do": true, "else": true, "elsif": true, "eval": true,
			"for": true, "foreach": true, "given": true, "goto": true, "if": true,
			"last": true, "local": true, "my": true, "next": true, "no": true,
			"our": true, "package": true, "redo": true, "require": true, "return": true,
			"state": true, "sub": true, "unless": true, "until": true, "use": true,
			"when": true, "while": true,
		},
	},
	"lua": {
		keywords: map[string]bool{
			"and": true, "break": true, "do": true, "else": true, "elseif": true,
			"end": true, "false": true, "for": true, "function": true, "goto": true,
			"if": true, "in": true, "local": true, "nil": true, "not": true,
			"or": true, "repeat": true, "return": true, "then": true, "true": true,
			"until": true, "while": true,
		},
		types: map[string]bool{
			"boolean": true, "number": true, "string": true, "table": true, "thread": true,
			"userdata": true,
		},
	},
	"r": {
		keywords: map[string]bool{
			"break": true, "else": true, "FALSE": true, "for": true, "function": true,
			"if": true, "in": true, "Inf": true, "NA": true, "NA_character_": true,
			"NA_complex_": true, "NA_integer_": true, "NA_real_": true, "NaN": true, "next": true,
			"NULL": true, "repeat": true, "TRUE": true, "while": true,
		},
		types: map[string]bool{
			"character": true, "complex": true, "double": true, "integer": true, "list": true,
			"logical": true, "numeric": true, "raw": true,
		},
	},
	"julia": {
		keywords: map[string]bool{
			"baremodule": true, "begin": true, "break": true, "catch": true, "const": true,
			"continue": true, "do": true, "else": true, "elseif": true, "end": true,
			"export": true, "finally": true, "for": true, "function": true, "global": true,
			"if": true, "import": true, "let": true, "local": true, "macro": true,
			"module": true, "mutable": true, "primitive": true, "quote": true, "return": true,
			"struct": true, "try": true, "using": true, "where": true, "while": true,
		},
		types: map[string]bool{
			"Any": true, "Array": true, "Bool": true, "Char": true, "Complex": true,
			"Float16": true, "Float32": true, "Float64": true, "Int": true, "Int32": true,
			"Int64": true, "Nothing": true, "String": true, "UInt": true, "Vector": true,
		},
	},
	"objc": {
		keywords: map[string]bool{
			"autoreleasepool": true, "break": true, "case": true, "const": true, "continue": true,
			"default": true, "do": true, "else": true, "end": true, "enum": true,
			"extern": true, "for": true, "goto": true, "if": true, "implementation": true,
			"import": true, "inline": true, "interface": true, "property": true, "protocol": true,
			"return": true, "sizeof": true, "static": true, "struct": true, "switch": true,
			"synthesize": true, "typedef": true, "union": true, "volatile": true, "while": true,
		},
		types: map[string]bool{
			"BOOL": true, "char": true, "double": true, "float": true, "id": true,
			"int": true, "long": true, "SEL": true, "short": true, "signed": true,
			"unsigned": true, "void": true,
		},
	},
	"matlab": {
		keywords: map[string]bool{
			"arguments": true, "break": true, "case": true, "catch": true, "classdef": true,
			"continue": true, "else": true, "elseif": true, "end": true, "enumeration": true,
			"events": true, "for": true, "function": true, "global": true, "if": true,
			"methods": true, "otherwise": true, "parfor": true, "persistent": true, "properties": true,
			"return": true, "spmd": true, "switch": true, "try": true, "while": true,
		},
		types: map[string]bool{
			"cell": true, "char": true, "double": true, "int8": true, "int16": true,
			"int32": true, "int64": true, "logical": true, "single": true, "string": true,
			"struct": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		},
	},
	"bash": {
		keywords: map[string]bool{
			"case": true, "coproc": true, "do": true, "done": true, "elif": true,
			"else": true, "esac": true, "fi": true, "for": true, "function": true,
			"if": true, "in": true, "select": true, "then": true, "time": true,
			"until": true, "while": true,
		},
	},
	"sql": {
		foldCase: true,
		keywords: map[string]bool{
			"ALL": true, "ALTER": true, "AND": true, "AS": true, "BEGIN": true,
			"BETWEEN": true, "BY": true, "CASE": true, "COMMIT": true, "CREATE": true,
			"DELETE": true, "DISTINCT": true, "DROP": true, "ELSE": true, "END": true,
			"EXISTS": true, "FROM": true, "FULL": true, "GROUP": true, "HAVING": true,
			"IN": true, "INNER": true, "INSERT": true, "INTO": true, "IS": true,
			"JOIN": true, "LEFT": true, "LIKE": true, "LIMIT": true, "NOT": true,
			"NULL": true, "ON": true, "OR": true, "ORDER": true, "OUTER": true,
			"OVER": true, "RIGHT": true, "ROLLBACK": true, "SELECT": true, "SET": true,
			"TABLE": true, "THEN": true, "UNION": true, "UPDATE": true, "VALUES": true,
			"WHEN": true, "WHERE": true, "WITH": true,
		},
		types: map[string]bool{
			"BIGINT": true, "BINARY": true, "BOOLEAN": true, "CHAR": true, "DATE": true,
			"DECIMAL": true, "DOUBLE": true, "FLOAT": true, "INT": true, "INTEGER": true,
			"NUMERIC": true, "REAL": true, "SMALLINT": true, "TEXT": true, "TIME": true,
			"TIMESTAMP": true, "VARCHAR": true,
		},
	},
	"latex": {
		keywords: map[string]bool{
			"begin": true, "end": true, "documentclass": true, "usepackage": true, "newcommand": true,
			"renewcommand": true, "newenvironment": true, "renewenvironment": true, "author": true, "title": true,
			"date": true, "maketitle": true, "chapter": true, "section": true, "subsection": true,
			"subsubsection": true, "paragraph": true, "label": true, "ref": true, "cite": true,
			"caption": true, "include": true, "includegraphics": true, "input": true, "item": true,
			"textbf": true, "textit": true,
		},
	},
	"clojure": {
		keywords: map[string]bool{
			"def": true, "do": true, "fn": true, "if": true, "let": true,
			"loop": true, "recur": true, "quote": true, "var": true, "throw": true,
			"try": true, "catch": true, "finally": true, "new": true, "set": true,
			"ns": true, "in-ns": true, "defn": true, "defmacro": true, "defprotocol": true,
			"deftype": true, "defrecord": true, "reify": true, "case": true,
		},
		types: map[string]bool{
			"Boolean": true, "Character": true, "Double": true, "Float": true, "Long": true,
			"Number": true, "Object": true, "String": true,
		},
	},
	"haskell": {
		keywords: map[string]bool{
			"case": true, "class": true, "data": true, "default": true, "deriving": true,
			"do": true, "else": true, "foreign": true, "if": true, "import": true,
			"in": true, "infix": true, "infixl": true, "infixr": true, "instance": true,
			"let": true, "module": true, "newtype": true, "of": true, "then": true,
			"type": true, "where": true,
		},
		types: map[string]bool{
			"Bool": true, "Char": true, "Double": true, "Either": true, "Float": true,
			"Int": true, "Integer": true, "Maybe": true, "Ordering": true, "String": true,
			"Word": true,
		},
	},
	"yaml": {},
	"html": {},
	"xml":  {},
	"text": {},
}
