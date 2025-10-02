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

## CLI-Optionen

Wichtige Flags:

- `-config <Pfad>`: Optionen aus einer YAML-Datei laden (Standard: `linkcheck.yaml`, sofern vorhanden)
- `-print-config`: effektive Konfiguration als YAML ausgeben und beenden
- `-healthcheck`: Einzelnen Healthcheck ausführen und JSON ausgeben (Exit-Code 1 bei Fehlern)
- `-healthcheck-file <Pfad>`: Healthcheck-Modus für alle zeilenweise aufgelisteten URLs in der Datei ausführen
- `-healthcheck-interval <Dauer>`: Healthcheck-Modus in dem angegebenen Intervall wiederholen (z. B. `30s`); bricht beim ersten Fehler ab
- `-e`: zusätzlich externe Links validieren
- `-workers`, `-timeout`, `-max-links`, `-rpm`, `-allow-ext`: Crawl-Verhalten feinjustieren
- `-cache`: Pfad zur Cache-Datei festlegen (Standard `.linkcheck-cache.json`)

Mit `linkcheck -h` lässt sich die vollständige Flag-Liste anzeigen.

Werden Flags und YAML gleichzeitig genutzt, überschreiben die Flags die Werte aus der Datei.

## YAML-Konfiguration

`linkcheck` liest eine YAML-Datei, wenn `-config` gesetzt ist (Standard `linkcheck.yaml`). Alle Optionen besitzen sinnvolle Default-Werte, daher sind leere oder fehlende Dateien erlaubt. Beispiel:

```yaml
start_url: https://example.com/
allow_external: false
workers: 8
timeout: 15s
max_links: 200
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

Mit `linkcheck -config linkcheck.web.yaml -print-config` lässt sich die effektive Konfiguration prüfen.

## Healthcheck-Modus

Der Healthcheck-Modus ist für Pipelines ausgelegt:

```bash
linkcheck -config linkcheck.web.yaml -healthcheck
```

- Gibt ein einzelnes JSON-Objekt mit Status, HTTP-Code, Dauer und gesammelten Fehlern aus
- Unterdrückt Fortschrittsmeldungen und beendet sich mit Exit-Code `1` bei Problemen
- Respektiert weiterhin Rate Limits und robots.txt

Die JSON-Ausgabe kann in CI/CD-Pipelines ausgewertet werden, und Fehlermeldungen erleichtern die Analyse.

Mehrere URLs lassen sich mit einer Datei prüfen, die pro Zeile eine Adresse enthält:

```bash
linkcheck -healthcheck -healthcheck-file urls.txt
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

Durch Kombination von `-healthcheck` und `-healthcheck-interval` lässt sich eine URL (oder Liste von URLs) regelmäßig prüfen:

```bash
linkcheck -healthcheck -healthcheck-interval 1m https://example.com
```

Nach jedem Durchlauf wird JSON ausgegeben, anschließend wartet das Tool für die angegebene Dauer. Sobald ein Durchlauf fehlschlägt, beendet sich der Prozess mit Exit-Code `1` – ideal für Watchdog-Skripte oder Container-Liveness-Prüfungen.

## Entwicklung

- Build: `go build ./...`
- Tests: `GOCACHE=$PWD/.gocache go test ./...`
- Manueller Lauf: `go run ./cmd/linkcheck -rpm 60 https://example.com`

Vor Commits stets `gofmt -w` auf geänderte Go-Dateien anwenden.

## Lizenz

Standardmäßig wird von einer MIT-ähnlichen Lizenz ausgegangen; bitte anpassen, falls das Projekt eine andere Lizenz verwendet.
