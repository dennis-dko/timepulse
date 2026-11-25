package test

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestEverySignInScreenDrawsTheSameSky holds the sky behind the sign-in card and
// the one behind the installer to one drawing. The two pages carry a copy each of
// the script, the svg and the rules, because the installer is one self-contained
// file served before there is anything else to load. This test is what keeps
// the copies one: the same table of colours through the day, the same sunrise
// and sunset, the same code, the same drawing, the same rules.
//
// Two copies of one picture drift apart a number at a time, and a sky that is
// rose in the evening on one screen and blue on the next is a mistake nobody
// would think to report.
func TestEverySignInScreenDrawsTheSameSky(t *testing.T) {
	app := read(t, "../internal/interface/web/asset/app.js")
	page := read(t, "../internal/interface/web/asset/index.html")
	style := read(t, "../internal/interface/web/asset/app.css")
	installer := read(t, "../internal/interface/installer/asset/install.html")

	for _, name := range []string{"SKY", "SUNRISE", "SUNSET", "skyColourAt", "drawTheSky"} {
		if want, got := scriptDeclaration(t, app, name), scriptDeclaration(t, installer, name); got != want {
			t.Errorf("the installer's %s is not the sign-in screen's:\n  sign-in:   %s\n  installer: %s", name, want, got)
		}
	}

	if want, got := skyDrawing(t, page, "login-sky"), skyDrawing(t, installer, "install-sky"); got != want {
		t.Errorf("the installer's sky is not drawn as the sign-in screen's:\n  sign-in:   %s\n  installer: %s", want, got)
	}

	want, got := skyRules(style), skyRules(installer)
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Errorf("the installer's rules for the sky are not the sign-in screen's:\n  sign-in:   %q\n  installer: %q", want, got)
	}

	if len(want) < 10 {
		t.Fatalf("found %d rules for the sky in app.css, so this compares too little", len(want))
	}

	darkApp := regexp.MustCompile(`:root\[data-theme="dark"\] \.sky \{([^}]*)\}`).FindStringSubmatch(style)
	darkInstaller := regexp.MustCompile(`@media \(prefers-color-scheme: dark\) \{\s*\.sky \{([^}]*)\}`).FindStringSubmatch(installer)

	switch {
	case darkApp == nil || darkInstaller == nil:
		t.Errorf("a dark theme's sky is missing: in app.css %v, in the installer %v", darkApp != nil, darkInstaller != nil)
	case declarations(darkApp[1]) != declarations(darkInstaller[1]):
		t.Errorf("the dark theme's sky differs: app.css %q, the installer %q", darkApp[1], darkInstaller[1])
	}
}

// TestTheConnectScreenDrawsTheSignInSky holds the Android app's connect screen to
// the same sky. Kotlin draws it on a canvas rather than from an svg, so what is
// compared is what the drawing is made of: the colours through the day, the day
// itself, how strongly the colour and the stars show on either ground, the sun's
// and the glow's colours, the stars and the ticks on the arc.
func TestTheConnectScreenDrawsTheSignInSky(t *testing.T) {
	app := read(t, "../internal/interface/web/asset/app.js")
	page := read(t, "../internal/interface/web/asset/index.html")
	style := read(t, "../internal/interface/web/asset/app.css")
	kotlin := read(t, "../internal/interface/mobile/src/main/kotlin/io/github/dennisdko/timepulse/Sky.kt")

	var web, phone []string

	for _, stop := range regexp.MustCompile(`\[([0-9.]+), '#([0-9a-f]{6})'\]`).FindAllStringSubmatch(scriptDeclaration(t, app, "SKY"), -1) {
		web = append(web, stop[1]+" "+stop[2])
	}

	for _, stop := range regexp.MustCompile(`([0-9.]+)f to 0xFF([0-9A-F]{6})`).FindAllStringSubmatch(kotlin, -1) {
		phone = append(phone, stop[1]+" "+strings.ToLower(stop[2]))
	}

	if len(web) < 5 || strings.Join(web, ", ") != strings.Join(phone, ", ") {
		t.Errorf("the app's colours through the day are not the sign-in screen's:\n  sign-in: %v\n  app:     %v", web, phone)
	}

	base := regexp.MustCompile(`(?m)^\.sky \{([^}]*)\}`).FindStringSubmatch(style)
	dark := regexp.MustCompile(`:root\[data-theme="dark"\] \.sky \{([^}]*)\}`).FindStringSubmatch(style)
	night := regexp.MustCompile(`\.sky\.is-night \{([^}]*)\}`).FindStringSubmatch(style)

	if base == nil || dark == nil || night == nil {
		t.Fatalf("app.css has no rule for the sky, its dark theme or its night: %v %v %v", base != nil, dark != nil, night != nil)
	}

	for _, same := range []struct{ kotlin, web string }{
		{kotlinConstant(t, kotlin, "SUNRISE"), jsConstant(t, app, "SUNRISE")},
		{kotlinConstant(t, kotlin, "SUNSET"), jsConstant(t, app, "SUNSET")},
		{kotlinConstant(t, kotlin, "LIGHT_STRENGTH"), cssValue(t, base[1], "--sky-strength")},
		{kotlinConstant(t, kotlin, "DARK_STRENGTH"), cssValue(t, dark[1], "--sky-strength")},
		{kotlinConstant(t, kotlin, "LIGHT_STARS"), cssValue(t, base[1], "--star-strength")},
		{kotlinConstant(t, kotlin, "DARK_STARS"), cssValue(t, dark[1], "--star-strength")},
		{kotlinConstant(t, kotlin, "LIGHT_SUN"), cssValue(t, base[1], "--sun")},
		{kotlinConstant(t, kotlin, "DARK_SUN"), cssValue(t, dark[1], "--sun")},
		{kotlinConstant(t, kotlin, "DAY_HALO"), cssValue(t, base[1], "--halo")},
		{kotlinConstant(t, kotlin, "NIGHT_HALO"), cssValue(t, night[1], "--halo")},
	} {
		if same.kotlin != same.web {
			t.Errorf("the app draws %s where the sign-in screen draws %s", same.kotlin, same.web)
		}
	}

	drawing := skyDrawing(t, page, "login-sky")

	for _, group := range []struct{ class, list, kotlin string }{
		{"sky-stars", "stars", `Triple\(([0-9.]+)f, ([0-9.]+)f, ([0-9.]+)f\)`},
		{"sky-ticks", "ticks", `Offset\(([0-9.]+)f, ([0-9.]+)f\)`},
	} {
		circles := regexp.MustCompile(`<g class="` + group.class + `">(.*?)</g>`).FindStringSubmatch(drawing)
		if circles == nil {
			t.Fatalf("the sign-in screen's sky has no %s", group.class)
		}

		var web, phone []string

		for _, circle := range regexp.MustCompile(`<circle cx="([0-9.]+)" cy="([0-9.]+)" r="([0-9.]+)"/>`).FindAllStringSubmatch(circles[1], -1) {
			if group.class == "sky-stars" {
				web = append(web, circle[1]+","+circle[2]+","+circle[3])
			} else {
				web = append(web, circle[1]+","+circle[2])
			}
		}

		// Read from the list of that name only: the drawing below it writes points
		// of its own, the horizon's ends among them.
		list := regexp.MustCompile(`(?s)val ` + group.list + ` = listOf\((.*?)\n    \)`).FindStringSubmatch(kotlin)
		if list == nil {
			t.Fatalf("Sky.kt declares no list of %s", group.list)
		}

		for _, point := range regexp.MustCompile(group.kotlin).FindAllStringSubmatch(list[1], -1) {
			phone = append(phone, strings.Join(point[1:], ","))
		}

		if len(web) == 0 || strings.Join(web, " ") != strings.Join(phone, " ") {
			t.Errorf("the app's %s are not the sign-in screen's:\n  sign-in: %v\n  app:     %v", group.class, web, phone)
		}
	}
}

// kotlinConstant is the value of a const val in Kotlin, written as the web
// writes it: a number without its f, or a colour as #rrggbb.
func kotlinConstant(t *testing.T, kotlin, name string) string {
	t.Helper()

	value := regexp.MustCompile(`const val ` + name + ` = (\S+)`).FindStringSubmatch(kotlin)
	if value == nil {
		t.Fatalf("Sky.kt declares no %s", name)
	}

	if colour, ok := strings.CutPrefix(value[1], "0xFF"); ok {
		return "#" + strings.ToLower(colour)
	}

	return strings.TrimSuffix(value[1], "f")
}

// jsConstant is the value of a const in a script.
func jsConstant(t *testing.T, js, name string) string {
	t.Helper()

	value := regexp.MustCompile(`const ` + name + ` = ([^;]+);`).FindStringSubmatch(js)
	if value == nil {
		t.Fatalf("app.js declares no %s", name)
	}

	return value[1]
}

// cssValue is the value of a declaration in a rule's body.
func cssValue(t *testing.T, body, property string) string {
	t.Helper()

	value := regexp.MustCompile(regexp.QuoteMeta(property) + `:\s*([^;]+);`).FindStringSubmatch(body)
	if value == nil {
		t.Fatalf("the rule sets no %s: %q", property, body)
	}

	return strings.TrimSpace(value[1])
}

// scriptDeclaration is a const or a function of a script, with its spacing evened out:
// `const NAME = ...;` up to the semicolon that ends it, or `function NAME(...)`
// up to the brace that closes its body.
func scriptDeclaration(t *testing.T, source, name string) string {
	t.Helper()

	start := regexp.MustCompile(`(?m)^\s*(?:const ` + name + ` = |function ` + name + `\()`).FindStringIndex(source)
	if start == nil {
		t.Fatalf("%s is declared nowhere this test reads", name)
	}

	depth, opened := 0, false

	for i := start[0]; i < len(source); i++ {
		switch source[i] {
		case '{', '[', '(':
			depth++
			opened = opened || source[i] == '{'
		case '}', ']', ')':
			depth--

			if depth == 0 && opened && source[i] == '}' && strings.HasPrefix(strings.TrimSpace(source[start[0]:i]), "function") {
				return evened(source[start[0] : i+1])
			}
		case ';':
			if depth == 0 && !strings.HasPrefix(strings.TrimSpace(source[start[0]:i]), "function") {
				return evened(source[start[0] : i+1])
			}
		}
	}

	t.Fatalf("%s is never closed", name)

	return ""
}

// skyDrawing is the svg with the id given, its comments left out and its ids
// written as the page-neutral "sky".
func skyDrawing(t *testing.T, source, id string) string {
	t.Helper()

	drawing := regexp.MustCompile(`(?s)<svg class="sky" id="` + id + `".*?</svg>`).FindString(source)
	if drawing == "" {
		t.Fatalf("there is no svg.sky with the id %s", id)
	}

	drawing = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(drawing, "")

	return evened(strings.ReplaceAll(drawing, id, "sky"))
}

// skyRules is every rule of a stylesheet whose selector starts with .sky,
// outside the dark theme's, each written one way: its selector's spacing evened
// out and its declarations sorted.
func skyRules(css string) []string {
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	css = regexp.MustCompile(`@media \(prefers-color-scheme: dark\) \{[^{}]*\{[^}]*\}\s*\}`).ReplaceAllString(css, "")

	var rules []string

	for _, rule := range regexp.MustCompile(`(?m)^\s*(\.sky[^{]*)\{([^}]*)\}`).FindAllStringSubmatch(css, -1) {
		rules = append(rules, evened(rule[1])+" { "+declarations(rule[2])+" }")
	}

	return rules
}

// declarations is a rule's body with each declaration trimmed, in sorted order.
func declarations(body string) string {
	var each []string

	for _, declaration := range strings.Split(body, ";") {
		if declaration = evened(declaration); declaration != "" {
			each = append(each, declaration)
		}
	}

	sort.Strings(each)

	return strings.Join(each, "; ")
}

// evened is text with every run of spacing written as one space.
func evened(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
