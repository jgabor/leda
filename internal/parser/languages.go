package parser

import (
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

var defaultLanguages = []langConfig{
	{
		name:       "go",
		extensions: []string{".go"},
		langPtr:    tree_sitter_go.Language(),
		imports:    `(import_spec path: (interpreted_string_literal (interpreted_string_literal_content) @path))`,
		symbols: `
			(function_declaration name: (identifier) @name (#match? @name "^[A-Z]"))
			(method_declaration name: (field_identifier) @name (#match? @name "^[A-Z]"))
			(type_declaration (type_spec name: (type_identifier) @name (#match? @name "^[A-Z]")))
			(const_spec name: (identifier) @name (#match? @name "^[A-Z]"))
			(var_spec name: (identifier) @name (#match? @name "^[A-Z]"))
		`,
	},
	{
		name:       "typescript",
		extensions: []string{".ts", ".tsx"},
		langPtr:    tree_sitter_typescript.LanguageTypescript(),
		imports: `
			(import_statement source: (string (string_fragment) @path))
			(export_statement source: (string (string_fragment) @path))
			(call_expression
				function: (identifier) @_fn
				arguments: (arguments (string (string_fragment) @path))
				(#eq? @_fn "require"))
		`,
		symbols: `
			(export_statement declaration: (function_declaration name: (identifier) @name))
			(export_statement declaration: (class_declaration name: (type_identifier) @name))
			(export_statement declaration: (interface_declaration name: (type_identifier) @name))
			(export_statement declaration: (type_alias_declaration name: (type_identifier) @name))
			(export_statement declaration: (lexical_declaration (variable_declarator name: (identifier) @name)))
			(export_statement declaration: (enum_declaration name: (identifier) @name))
		`,
	},
	{
		name:       "javascript",
		extensions: []string{".js", ".jsx", ".mjs"},
		langPtr:    tree_sitter_javascript.Language(),
		imports: `
			(import_statement source: (string (string_fragment) @path))
			(export_statement source: (string (string_fragment) @path))
			(call_expression
				function: (identifier) @_fn
				arguments: (arguments (string (string_fragment) @path))
				(#eq? @_fn "require"))
		`,
		symbols: `
			(export_statement declaration: (function_declaration name: (identifier) @name))
			(export_statement declaration: (class_declaration name: (identifier) @name))
			(export_statement declaration: (lexical_declaration (variable_declarator name: (identifier) @name)))
		`,
	},
	{
		name:       "python",
		extensions: []string{".py"},
		langPtr:    tree_sitter_python.Language(),
		imports: `
			(import_statement name: (dotted_name) @path)
			(import_from_statement module_name: (dotted_name) @path)
			(import_from_statement module_name: (relative_import) @path)
		`,
		symbols: `
			(function_definition name: (identifier) @name)
			(class_definition name: (identifier) @name)
		`,
	},
	{
		name:       "rust",
		extensions: []string{".rs"},
		langPtr:    tree_sitter_rust.Language(),
		imports:    `(use_declaration argument: (_) @path)`,
		symbols: `
			(function_item name: (identifier) @name)
			(struct_item name: (type_identifier) @name)
			(enum_item name: (type_identifier) @name)
			(trait_item name: (type_identifier) @name)
			(type_item name: (type_identifier) @name)
		`,
	},
	{
		name:       "java",
		extensions: []string{".java"},
		langPtr:    tree_sitter_java.Language(),
		imports:    `(import_declaration (scoped_identifier) @path)`,
		symbols: `
			(class_declaration name: (identifier) @name)
			(interface_declaration name: (identifier) @name)
			(enum_declaration name: (identifier) @name)
			(method_declaration name: (identifier) @name)
		`,
	},
	{
		name:       "c",
		extensions: []string{".c", ".h"},
		langPtr:    tree_sitter_c.Language(),
		imports: `
			(preproc_include path: (string_literal) @path)
			(preproc_include path: (system_lib_string) @path)
		`,
		symbols: `
			(function_definition declarator: (function_declarator declarator: (identifier) @name))
			(declaration declarator: (function_declarator declarator: (identifier) @name))
			(type_definition declarator: (type_identifier) @name)
			(struct_specifier name: (type_identifier) @name)
			(enum_specifier name: (type_identifier) @name)
		`,
	},
	{
		name:       "cpp",
		extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"},
		langPtr:    tree_sitter_cpp.Language(),
		imports: `
			(preproc_include path: (string_literal) @path)
			(preproc_include path: (system_lib_string) @path)
		`,
		symbols: `
			(function_definition declarator: (function_declarator declarator: (identifier) @name))
			(class_specifier name: (type_identifier) @name)
			(struct_specifier name: (type_identifier) @name)
			(enum_specifier name: (type_identifier) @name)
			(namespace_definition name: (identifier) @name)
		`,
	},
	{
		name:       "ruby",
		extensions: []string{".rb"},
		langPtr:    tree_sitter_ruby.Language(),
		imports:    `(call method: (identifier) @_fn arguments: (argument_list (string (string_content) @path)) (#match? @_fn "^require"))`,
		symbols: `
			(class name: (constant) @name)
			(module name: (constant) @name)
			(method name: (identifier) @name)
		`,
	},
	{
		name:       "php",
		extensions: []string{".php"},
		langPtr:    tree_sitter_php.LanguagePHP(),
		imports: `
			(use_declaration (name) @path)
			(namespace_use_clause (name) @path)
		`,
		symbols: `
			(class_declaration name: (name) @name)
			(interface_declaration name: (name) @name)
			(trait_declaration name: (name) @name)
			(function_definition name: (name) @name)
			(method_declaration name: (name) @name)
		`,
	},
}
