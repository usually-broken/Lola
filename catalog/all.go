// Package catalog blank-imports every plugin so that their factories register.
//
// It is not a plugin: it has no manifest, it cannot be mounted, and it never
// appears in the tree. But something has to bring the plugins into the binary,
// because in Go a package's init only runs if something imports it — so this is
// the something, which is why it sits beside core and plugins rather than inside
// either of them.
//
// This is the only file that lists plugins at compile time. A new plugin is
// added here so the compiler includes it; which plugins actually mount is
// decided by lola.yaml, not by this file.
package catalog

import (
	// The catalog. Keep sorted.
	_ "github.com/usually-broken/lola/plugins/agentloop"
	_ "github.com/usually-broken/lola/plugins/composer"
	_ "github.com/usually-broken/lola/plugins/creator"
	_ "github.com/usually-broken/lola/plugins/deletefile"
	_ "github.com/usually-broken/lola/plugins/greet"
	_ "github.com/usually-broken/lola/plugins/guardchip"
	_ "github.com/usually-broken/lola/plugins/health"
	_ "github.com/usually-broken/lola/plugins/keepfile"
	_ "github.com/usually-broken/lola/plugins/listen"
	_ "github.com/usually-broken/lola/plugins/llmrouter"
	_ "github.com/usually-broken/lola/plugins/modechip"
	_ "github.com/usually-broken/lola/plugins/openai"
	_ "github.com/usually-broken/lola/plugins/promptaftertool"
	_ "github.com/usually-broken/lola/plugins/promptmode"
	_ "github.com/usually-broken/lola/plugins/promptpersona"
	_ "github.com/usually-broken/lola/plugins/promptpretool"
	_ "github.com/usually-broken/lola/plugins/readfile"
	_ "github.com/usually-broken/lola/plugins/runshell"
	_ "github.com/usually-broken/lola/plugins/sessionlist"
	_ "github.com/usually-broken/lola/plugins/sessions"
	_ "github.com/usually-broken/lola/plugins/settingsgeneral"
	_ "github.com/usually-broken/lola/plugins/settingspermissions"
	_ "github.com/usually-broken/lola/plugins/settingsproviders"
	_ "github.com/usually-broken/lola/plugins/settingsscreen"
	_ "github.com/usually-broken/lola/plugins/speak"
	_ "github.com/usually-broken/lola/plugins/speeds"
	_ "github.com/usually-broken/lola/plugins/standard"
	_ "github.com/usually-broken/lola/plugins/tree"
	_ "github.com/usually-broken/lola/plugins/tui"
	_ "github.com/usually-broken/lola/plugins/voice"
	_ "github.com/usually-broken/lola/plugins/web"
	_ "github.com/usually-broken/lola/plugins/webfetch"
	_ "github.com/usually-broken/lola/plugins/websearch"
	_ "github.com/usually-broken/lola/plugins/writefile"
)
