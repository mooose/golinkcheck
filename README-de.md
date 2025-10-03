# linkcheck

`linkcheck` ist ein höflicher, nebenläufiger Crawler zum Validieren von Links in Websites oder Webanwendungen. Das Tool wird als CLI ausgeliefert, kann eine Start-URL crawlen, interne Links folgen und erzeugt auf Wunsch maschinenlesbare Healthcheck-Ergebnisse für CI/CD-Pipelines.

## Funktionen

- Nebenläufiges Crawling mit Request-Drosselung und robots.txt-Unterstützung
- Healthcheck-Modus mit JSON-Ausgabe und Fehlerstatus für automatisierte Prüfungen
- YAML-Konfiguration mit umgebungsabhängigen Presets und überschreibbaren CLI-Flags
- Crawl-Cache für schnellere Wiederholungen sowie Filterung nach Dateiendungen

## Installation

```bash
go install ./cmd/linkcheck
```

Der Build benötigt Go 1.25.1 oder neuer.

## Schnellstart

```bash
linkcheck https://example.com
```

Standardmäßig respektiert der Crawl robots.txt, folgt bis zu 200 internen Seiten und begrenzt Anfragen auf 60 pro Minute.

## CLI-Übersicht

Die CLI nutzt [Kong](https://github.com/alecthomas/kong) und gruppiert verwandte Optionen, damit sich Erweiterungen leichter überblicken lassen. `linkcheck --help` zeigt eine hervorgehobene Zusammenfassung wie unten:

```
linkcheck [flags] START-URL

Konfiguration
  --config, -c DATEI          Pfad zu einer YAML-Konfigurationsdatei (Standard linkcheck.yaml). Leerer Wert deaktiviert das Laden.
  --print-config              Effektive Konfiguration als YAML ausgeben und beenden.

Crawl-Richtlinien
  --allow-external, -e        Externe Links in die Validierung einbeziehen.
  --workers N                 Anzahl gleichzeitiger Worker für interne Seiten (Standard 8).
  --timeout DAUER             HTTP-Timeout pro Anfrage (Standard 15s). Beispiele: 20s, 500ms.
  --max-links N               Maximale Anzahl interner Seiten, denen gefolgt wird (Standard 200).
  --max-depth N               Maximale Crawl-Tiefe ab Start-URL (-1 für unbegrenzt, Standard -1).
  --rpm N                     Maximale HTTP-Anfragen pro Minute inkl. robots.txt (Standard 60).
  --allow-ext LISTE           Kommagetrennte Endungen, denen gefolgt wird (Standard .html,.htm). Leerer Eintrag erlaubt pfadlose URLs.
  --ignore-robots             robots.txt ignorieren (nur für Tests).

Speicher & Reporting
  --cache DATEI               Pfad zur Crawl-Cache-Datei (Standard .linkcheck-cache.json).
  --markdown-dir VERZ         Verzeichnis für Markdown-Exporte (Standard .linkcheck-pages). Leer lassen, um zu deaktivieren.

Healthcheck
  --healthcheck               Einzelnen Healthcheck ausführen und CI-freundliches JSON erzeugen.
  --healthcheck-file DATEI    Zeilenweise URLs für wiederholte Healthchecks einlesen.
  --healthcheck-interval T    Healthcheck in diesem Intervall wiederholen (benötigt --healthcheck).

Meta
  --version                   Versionsinformationen ausgeben und beenden.

Positionsargumente sind optional, wenn `--healthcheck-file` gesetzt ist; andernfalls muss eine Start-URL angegeben werden. CLI-Flags überschreiben YAML-Werte immer.

## YAML-Konfiguration

`linkcheck` liest eine YAML-Datei, wenn `--config` gesetzt ist (Standard `linkcheck.yaml`). Alle Optionen besitzen sinnvolle Default-Werte, daher sind leere oder fehlende Dateien erlaubt. Beispiel:

```yaml
start_url: https://example.com/
allow_external: false
workers: 8
timeout: 15s
max_links: 200
max_depth: -1
requests_per_minute: 60
allowed_extensions:
  - .html
  - .htm
ignore_robots: false
cache_path: .linkcheck-cache.json
healthcheck: false
healthcheck_file: ""
healthcheck_interval: 0s
```

Beigefügte Presets:

- `linkcheck.local.yaml` – Entwicklungscrawl gegen `http://localhost:8080`
- `linkcheck.web.yaml` – konservative Einstellungen für öffentliche HTTPS-Seiten

Mit `linkcheck --config linkcheck.web.yaml --print-config` lässt sich die effektive Konfiguration prüfen.

## Healthcheck-Modus

Der Healthcheck-Modus ist für Pipelines ausgelegt:

```bash
linkcheck --config linkcheck.web.yaml --healthcheck
```

- Gibt ein einzelnes JSON-Objekt mit Status, HTTP-Code, Dauer und gesammelten Fehlern aus
- Unterdrückt Fortschrittsmeldungen und beendet sich mit Exit-Code `1` bei Problemen
- Respektiert weiterhin Rate Limits und robots.txt

Die JSON-Ausgabe kann in CI/CD-Pipelines ausgewertet werden, und Fehlermeldungen erleichtern die Analyse.

Mehrere URLs lassen sich mit einer Datei prüfen, die pro Zeile eine Adresse enthält:

```bash
linkcheck --healthcheck --healthcheck-file urls.txt
```

Die Ausgabe enthält ein JSON-Objekt mit einem Gesamtstatus sowie einer `results`-Liste mit einem Eintrag pro URL. Der Exit-Code ist nur dann `0`, wenn alle Einträge bestehen.

### Format der Healthcheck-Datei

- Einfacher Text, eine URL pro Zeile.
- Leere Zeilen oder Zeilen, die mit `#` beginnen, werden übersprungen.
- URLs ohne Schema werden beim Normalisieren standardmäßig zu `https://` ergänzt.

### Beispielausgabe für mehrere URLs

```json
{
  "status": "fail",
  "results": [
    {
      "url": "https://example.com",
      "status": "pass",
      "duration_ms": 124,
      "pages_visited": 1
    },
    {
      "url": "https://example.org/broken",
      "status": "fail",
      "duration_ms": 98,
      "pages_visited": 1,
      "errors": [
        {
          "message": "status 404"
        }
      ]
    }
  ]
}
```

### Kontinuierliche Healthchecks

Durch Kombination von `--healthcheck` und `--healthcheck-interval` lässt sich eine URL (oder Liste von URLs) regelmäßig prüfen:

```bash
linkcheck --healthcheck --healthcheck-interval 1m https://example.com
```

Nach jedem Durchlauf wird JSON ausgegeben, anschließend wartet das Tool für die angegebene Dauer. Sobald ein Durchlauf fehlschlägt, beendet sich der Prozess mit Exit-Code `1` – ideal für Watchdog-Skripte oder Container-Liveness-Prüfungen.

## Entwicklung

- Build: `go build ./...`
- Tests: `GOCACHE=$PWD/.gocache go test ./...`
- Manueller Lauf: `go run ./cmd/linkcheck --rpm 60 https://example.com`

Vor Commits stets `gofmt -w` auf geänderte Go-Dateien anwenden.

## Lizenz

Standardmäßig wird von einer MIT-ähnlichen Lizenz ausgegangen; bitte anpassen, falls das Projekt eine andere Lizenz verwendet.
