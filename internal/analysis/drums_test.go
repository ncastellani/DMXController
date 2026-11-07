package analysis

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncastellani/DMXController/internal/audio"
)

// drumSong monta uma música de 64 s a 120 BPM: notas e acordes o tempo todo
// e, de 16 a 48 s, bateria (bumbo nas batidas ímpares, caixa nas pares).
// Devolve a música e só a bateria.
func drumSong() (y, d []float64) {
	const dur, beat = 64.0, 0.5
	n := int(dur * SR)
	y, d = make([]float64, n), make([]float64, n)
	rnd := rand.New(rand.NewSource(1))
	add := func(dst []float64, at float64, length float64, f func(t float64) float64) {
		for i := int(at * SR); i < min(n, int((at+length)*SR)); i++ {
			dst[i] += f(float64(i)/SR - at)
		}
	}
	notes := []float64{220, 277.18, 329.63, 246.94}
	for b := 0; float64(b)*beat < dur; b++ {
		at := float64(b) * beat
		f := notes[(b/4)%len(notes)] * (1 + float64(b%3)) // nota que muda a cada batida
		add(y, at+0.25, 0.4, func(t float64) float64 { return 0.25 * math.Exp(-t/0.12) * math.Sin(2*math.Pi*f*t) })
		if at < 16 || at >= 48 {
			continue
		}
		if b%2 == 0 { // bumbo
			add(d, at, 0.3, func(t float64) float64 { return 0.9 * math.Exp(-t/0.06) * math.Sin(2*math.Pi*55*t) })
		} else { // caixa: ruído com corpo em 200 Hz
			add(d, at, 0.2, func(t float64) float64 {
				return 0.5 * math.Exp(-t/0.05) * (0.7*(rnd.Float64()*2-1) + 0.3*math.Sin(2*math.Pi*200*t))
			})
		}
	}
	for i := range y {
		y[i] += d[i]
	}
	return y, d
}

// TestAnalyzeDrums: com a bateria separada, os toques da parte com bateria
// são bumbo e caixa em cima da batida, a batida das partes é o nível real da
// bateria (nada onde ela não toca), a música se divide onde ela entra e sai,
// e a entrada dela é um pico.
func TestAnalyzeDrums(t *testing.T) {
	y, d := drumSong()
	a := AnalyzeDrums(y, d)
	if !a.Drums || len(a.HitKind) != len(a.Hits) || len(a.HitStrength) != len(a.Hits) {
		t.Fatalf("drums %v, %d toques, %d tipos, %d forças", a.Drums, len(a.Hits), len(a.HitKind), len(a.HitStrength))
	}
	var kick, snare, other, off int
	for i, h := range a.Hits {
		kind := a.HitKind[i]
		switch {
		case h > 17 && h < 47:
			switch kind {
			case kindKick:
				kick++
			case kindSnare:
				snare++
			default:
				other++
			}
			if kind != kindOther && math.Abs(h/0.5-math.Round(h/0.5)) > 0.12 { // a mais de 60 ms da batida
				off++
			}
		case h < 15 || h > 49:
			if kind != kindOther || a.HitStrength[i] > 0.3 {
				t.Errorf("toque em %.2f s, fora da bateria: tipo %c, força %.2f", h, kind, a.HitStrength[i])
			}
		}
	}
	if kick < 20 || snare < 20 || other > (kick+snare)/6 || off > 2 {
		t.Errorf("na parte com bateria: %d de bumbo, %d de caixa, %d de outros, %d fora da batida", kick, snare, other, off)
	}
	near := func(x float64) bool {
		for _, s := range a.Sections {
			if math.Abs(s.Start-x) <= 1 {
				return true
			}
		}
		return false
	}
	if !near(16) || !near(48) {
		t.Errorf("a música deveria se dividir onde a bateria entra (16 s) e sai (48 s): %+v", a.Sections)
	}
	for _, s := range a.Sections {
		switch {
		case s.Start >= 15 && s.End <= 49 && s.Pulse < 0.6:
			t.Errorf("parte com bateria com batida %.2f: %+v", s.Pulse, s)
		case (s.End <= 15 || s.Start >= 49) && s.Pulse != 0.01:
			t.Errorf("parte sem bateria com batida %.2f: %+v", s.Pulse, s)
		}
	}
	entry := false
	for _, ac := range a.Accents {
		switch {
		case math.Abs(ac.T-16) <= 0.6 && ac.V == 1:
			entry = true
		case ac.T > 20 && ac.T < 47:
			t.Errorf("bateria uniforme não tem pico: %+v", ac)
		}
	}
	if !entry {
		t.Errorf("a entrada da bateria deveria ser um pico: %+v", a.Accents)
	}

	// Música sem bateria (a separação devolve quase silêncio): nenhum toque de
	// bateria, nenhuma parte com batida, nenhum pico.
	y = y[:30*SR] // (um trecho basta para os dois casos abaixo)
	quiet := make([]float64, len(y))
	for i := range quiet {
		quiet[i] = 0.0005 * math.Sin(float64(i))
	}
	b := AnalyzeDrums(y, quiet)
	if !b.Drums || strings.Trim(b.HitKind, string(rune(kindOther))) != "" || len(b.Accents) != 0 {
		t.Errorf("sem bateria: tipos %q, picos %+v", strings.Trim(b.HitKind, "-"), b.Accents)
	}
	for _, s := range b.Sections {
		if s.Pulse != 0.01 {
			t.Errorf("sem bateria, batida %.2f em %+v", s.Pulse, s)
		}
	}
	// Sem o ouvido, a análise é a de antes.
	if c := Analyze(y); c.Drums || c.HitKind != "" || len(c.Hits) == 0 {
		t.Errorf("sem o ouvido: drums %v, tipos %q", c.Drums, c.HitKind)
	}
}

// TestMain deixa o próprio binário de teste fazer o papel do Python do ouvido
// (ver fakeEar): assim o teste roda igual no Linux, no Mac e no Windows.
func TestMain(m *testing.M) {
	if dir := os.Getenv("DMX_FAKE_EAR"); dir != "" {
		os.Exit(fakeEarMain(dir))
	}
	os.Exit(m.Run())
}

// fakeEarMain é o "Python" de mentira, chamado como o de verdade
// (python - <música> <saída>, com o script na entrada): avisa o andamento,
// falha se dir/falhar existe e devolve a própria música como "bateria".
func fakeEarMain(dir string) int {
	io.Copy(io.Discard, os.Stdin)
	fmt.Println("progresso 50")
	if _, err := os.Stat(filepath.Join(dir, "falhar")); err == nil {
		fmt.Fprintln(os.Stderr, "Traceback")
		fmt.Fprintln(os.Stderr, "RuntimeError: erro de teste")
		return 3
	}
	if f, err := os.OpenFile(filepath.Join(dir, "chamadas"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.WriteString("x\n")
		f.Close()
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		return 1
	}
	if err := os.WriteFile(os.Args[3], data, 0o644); err != nil {
		return 1
	}
	return 0
}

// fakeEar monta um ouvido de mentira: o "Python" do ambiente é uma cópia do
// binário de teste, que vira fakeEarMain com DMX_FAKE_EAR definido.
func fakeEar(t *testing.T) (*Separator, string) {
	t.Helper()
	dir := t.TempDir()
	py := pythonIn(dir)
	if err := os.MkdirAll(filepath.Dir(py), 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(py, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DMX_FAKE_EAR", dir)
	s := FindSeparator(dir)
	if s == nil || !filepath.IsAbs(s.Python) || !filepath.IsAbs(s.Cache) {
		t.Fatalf("FindSeparator: %+v", s)
	}
	return s, dir
}

// TestSeparator: a separação roda uma vez por música (pelo conteúdo do
// arquivo), mostra o andamento e, se falha, diz o motivo sem deixar lixo.
func TestSeparator(t *testing.T) {
	if FindSeparator(t.TempDir()) != nil {
		t.Fatal("pasta sem o ambiente não tem ouvido")
	}
	s, dir := fakeEar(t)
	wav := filepath.Join(t.TempDir(), "m.wav")
	os.WriteFile(wav, wav16k(make([]float64, SR)), 0o644)
	var stages []string
	out, err := s.Drums(context.Background(), wav, func(st string) { stages = append(stages, st) })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); len(got) < 1000 || len(stages) != 1 || stages[0] != "separando a bateria (50%)" {
		t.Fatalf("saída de %d bytes, etapas %v", len(got), stages)
	}
	again, err := s.Drums(context.Background(), wav, nil)
	calls, _ := os.ReadFile(filepath.Join(dir, "chamadas"))
	if err != nil || again != out || strings.Count(string(calls), "x") != 1 {
		t.Errorf("a mesma música deveria vir do cache: %v, %q, chamadas %q", err, again, calls)
	}
	// Uma cópia com outro nome é a mesma música; outro conteúdo, não.
	copyPath := filepath.Join(t.TempDir(), "outro nome.wav")
	data, _ := os.ReadFile(wav)
	os.WriteFile(copyPath, data, 0o644)
	if same, _ := s.Drums(context.Background(), copyPath, nil); same != out {
		t.Errorf("cópia da música fora do cache: %q", same)
	}
	other := filepath.Join(t.TempDir(), "x.wav")
	os.WriteFile(other, wav16k(make([]float64, 2*SR)), 0o644)
	os.WriteFile(filepath.Join(dir, "falhar"), nil, 0o644)
	if _, err := s.Drums(context.Background(), other, nil); err == nil || !strings.Contains(err.Error(), "erro de teste") {
		t.Errorf("falha: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(s.Cache, "*")); len(left) != 1 {
		t.Errorf("a falha deixou arquivos no cache: %v", left)
	}
}

// TestGenerateWithEar: a geração usa a bateria separada e, se a separação
// falha, avisa e segue sem ela.
func TestGenerateWithEar(t *testing.T) {
	y, _ := drumSong()
	wav := filepath.Join(t.TempDir(), "m.wav")
	os.WriteFile(wav, wav16k(y[:30*SR]), 0o644)
	s, dir := fakeEar(t) // devolve a própria música como "bateria"
	var stages []string
	tl, warn, err := Generate(context.Background(), wav, catalog(t), Options{Separator: s, Progress: func(st string) { stages = append(stages, st) }})
	if err != nil || warn != "" || !tl.Analysis.Drums || !strings.Contains(string(tl.Generated), `"drums":true`) || !strings.Contains(strings.Join(stages, "|"), "separando a bateria (50%)") {
		t.Fatalf("com o ouvido: %v, aviso %q, drums %v, etapas %v", err, warn, tl.Analysis.Drums, stages)
	}
	os.Remove(mustGlob(t, filepath.Join(s.Cache, "*")))
	os.WriteFile(filepath.Join(dir, "falhar"), nil, 0o644)
	tl, warn, err = Generate(context.Background(), wav, catalog(t), Options{Separator: s})
	if err != nil || tl.Analysis.Drums || !strings.Contains(warn, "ouvido de bateria") || !strings.Contains(warn, "erro de teste") || len(tl.Cues) == 0 {
		t.Fatalf("com o ouvido falhando: %v, aviso %q, drums %v", err, warn, tl.Analysis.Drums)
	}
}

func mustGlob(t *testing.T, pattern string) string {
	t.Helper()
	m, _ := filepath.Glob(pattern)
	if len(m) != 1 {
		t.Fatalf("%s: %v", pattern, m)
	}
	return m[0]
}

// TestOuvidoReal usa o ouvido de bateria instalado de verdade (DMX_OUVIDO =
// a pasta dele, ex.: .ouvido): separa a bateria de uma música sintética com
// trecho sem bateria e confere que a bateria separada fica onde ela toca.
// Serve para conferir uma instalação (no Windows, scripts/instalar_ouvido.ps1).
func TestOuvidoReal(t *testing.T) {
	dir := os.Getenv("DMX_OUVIDO")
	if dir == "" {
		t.Skip("defina DMX_OUVIDO com a pasta do ouvido de bateria")
	}
	s := FindSeparator(dir)
	if s == nil {
		t.Fatalf("ouvido de bateria não encontrado em %s", dir)
	}
	s.Cache = t.TempDir() // não mexe no cache de verdade
	y, _ := drumSong()
	y = y[8*SR : 40*SR] // 8 s sem bateria e 24 s com
	wav := filepath.Join(t.TempDir(), "musica.wav")
	if err := os.WriteFile(wav, wav16k(y), 0o644); err != nil {
		t.Fatal(err)
	}
	var stages []string
	out, err := s.Drums(context.Background(), wav, func(st string) { stages = append(stages, st) })
	if err != nil {
		t.Fatal(err)
	}
	tr, err := audio.LoadWAV(out)
	if err != nil {
		t.Fatal(err)
	}
	d := Mono(tr)
	rms := func(from, to float64) float64 {
		var sum float64
		a, b := int(from*SR), min(len(d), int(to*SR))
		for _, v := range d[a:b] {
			sum += v * v
		}
		return math.Sqrt(sum / float64(max(1, b-a)))
	}
	without, with := rms(0.5, 7.5), rms(9, 31)
	t.Logf("bateria separada: RMS %.4f sem bateria, %.4f com; %d avisos de andamento", without, with, len(stages))
	if with < 0.02 || with < 5*without {
		t.Errorf("a bateria separada deveria ficar onde ela toca: RMS %.4f sem, %.4f com", without, with)
	}
	if a := AnalyzeDrums(y, d); !a.Drums || !strings.ContainsAny(a.HitKind, "ks") {
		t.Errorf("análise com a bateria separada: drums %v, peças %q", a.Drums, a.HitKind)
	}
}
