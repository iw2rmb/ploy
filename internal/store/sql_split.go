package store

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// execMigrationSQL splits the provided SQL into statements and executes them
// sequentially within the provided transaction. It supports semicolon-delimited
// statements and accounts for comments and common quoting modes (single quotes,
// double quotes, and dollar-quoted strings) so that semicolons inside those
// constructs do not split statements.
//
//nolint:unused // exported for future migration helpers; currently unused
func execMigrationSQL(ctx context.Context, tx pgx.Tx, sql string) error {
	stmts := splitSQLStatements(sql)
	for _, s := range stmts {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// splitSQLStatements splits a SQL script into individual statements by
// semicolons, ignoring semicolons that appear inside string literals,
// identifiers, dollar-quoted strings, or comments. Returned statements do not
// include the terminating semicolon.
func splitSQLStatements(script string) []string {
	var stmts []string
	var b strings.Builder

	inSingle := false    // '...'
	inDouble := false    // "..."
	inLineC := false     // -- ... \n
	inBlockC := false    // /* ... */
	inDollar := false    // $tag$ ... $tag$ or $$ ... $$
	var dollarTag string // tag part between the $...

	// helper to flush current buffer to statements
	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			stmts = append(stmts, s)
		}
		b.Reset()
	}

	// Iterate runes to properly handle multibyte tags
	rs := []rune(script)
	for i := 0; i < len(rs); i++ {
		r := rs[i]

		// Handle end of line for line comments
		if inLineC {
			b.WriteRune(r)
			if r == '\n' {
				inLineC = false
			}
			continue
		}

		// Handle block comments
		if inBlockC {
			b.WriteRune(r)
			if r == '*' && i+1 < len(rs) && rs[i+1] == '/' {
				b.WriteRune('/')
				i++
				inBlockC = false
			}
			continue
		}

		// Inside dollar-quoted string
		if inDollar {
			b.WriteRune(r)
			if r == '$' {
				if dollarTag == "" {
					// tagless $$ ... $$
					if i+1 < len(rs) && rs[i+1] == '$' {
						b.WriteRune('$')
						i++
						inDollar = false
					}
				} else {
					// Try to match closing $tag$
					tag := []rune(dollarTag)
					n := len(tag)
					if i+1+n < len(rs) { // need $ + tag + $
						match := true
						for j := 0; j < n; j++ {
							if rs[i+1+j] != tag[j] {
								match = false
								break
							}
						}
						if match && rs[i+1+n] == '$' {
							// consume tag and trailing $
							for j := 0; j < n+1; j++ {
								b.WriteRune(rs[i+1+j])
							}
							i += n + 1
							inDollar = false
							dollarTag = ""
						}
					}
				}
			}
			continue
		}

		// Not in any special region
		switch r {
		case '\'', '"':
			b.WriteRune(r)
			if r == '\'' && !inDouble {
				if inSingle {
					// Check for escaped ''
					if i+1 < len(rs) && rs[i+1] == '\'' {
						// stay in single quote, write the escape
						b.WriteRune('\'')
						i++
					} else {
						inSingle = false
					}
				} else if !inSingle {
					inSingle = true
				}
				continue
			}
			if r == '"' && !inSingle {
				inDouble = !inDouble
				continue
			}
		case '-':
			// Start of line comment? "--"
			if !inSingle && !inDouble && i+1 < len(rs) && rs[i+1] == '-' {
				b.WriteString("--")
				i++
				inLineC = true
				continue
			}
			b.WriteRune(r)
			continue
		case '/':
			// Start of block comment? "/*"
			if !inSingle && !inDouble && i+1 < len(rs) && rs[i+1] == '*' {
				b.WriteString("/*")
				i++
				inBlockC = true
				continue
			}
			b.WriteRune(r)
			continue
		case '$':
			// Dollar-quoted string start? $tag$
			if !inSingle && !inDouble {
				// Handle tagless $$
				if i+1 < len(rs) && rs[i+1] == '$' {
					b.WriteString("$$")
					i++
					inDollar = true
					dollarTag = ""
					continue
				}
				// collect tag characters (letters, digits, underscore)
				j := i + 1
				var tag strings.Builder
				for j < len(rs) && ((rs[j] >= 'a' && rs[j] <= 'z') || (rs[j] >= 'A' && rs[j] <= 'Z') || (rs[j] >= '0' && rs[j] <= '9') || rs[j] == '_') {
					tag.WriteRune(rs[j])
					j++
				}
				if j < len(rs) && rs[j] == '$' { // found $tag$
					// write the full $tag$
					b.WriteRune('$')
					for k := i + 1; k <= j; k++ {
						b.WriteRune(rs[k])
					}
					i = j
					inDollar = true
					dollarTag = tag.String()
					continue
				}
			}
			b.WriteRune(r)
			continue
		case ';':
			// Only split when not inside quotes
			if !inSingle && !inDouble {
				// finalize current statement without the semicolon
				flush()
				continue
			}
			b.WriteRune(r)
			continue
		}

		// default: append rune
		b.WriteRune(r)
	}

	// flush remaining
	flush()
	return stmts
}
