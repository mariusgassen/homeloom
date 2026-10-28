# homeloom

[English](README.md) · **Deutsch**

Ein gemeinsames Zuhause für zwei. Homeloom verwebt eure Mahlzeiten, Momente und euer Smart Home zu einem privaten, selbst gehosteten Ort für Paare und Haushalte.

## Repository

- `apps/api`: die Go-API (hexagonal, API first). Der Vertrag liegt in `apps/api/api/openapi.yaml`.
- `apps/web`: die PWA (Vite, React, TypeScript), auf dem iPhone über Safari installierbar.
- `apps/agent`: Homeloom als Stimme in Anrufen (LiveKit Agents für Node).
- `docs/architecture.md`: Entscheidungen, Aufbau und Deployment (auf Englisch).
- `docker-compose.yml`: der Stack, den Coolify ausrollt (`homeloom-web`, `homeloom-api`, `homeloom-livekit` für Anrufe, `homeloom-agent`, `homeloom-postgres`, `homeloom-redis`).

Die App spricht Deutsch und Englisch. Die API antwortet in der Sprache aus dem `Accept-Language`-Header.

## Lokal starten

```sh
cp .env.example .env   # POSTGRES_PASSWORD, PUBLIC_URL=http://localhost:3000, die authentik-Werte
                       # und LIVEKIT_API_SECRET (32+ Zeichen) für Anrufe setzen
                       # optional: MEALIE_MCP_URL + MEALIE_MCP_TOKEN zum Kochen, ANTHROPIC_API_KEY für Claude und den Assistenten
                       # optional: OPENAI_API_KEY (mit ANTHROPIC_API_KEY), um im Anruf mit Homeloom zu sprechen
                       # optional: HOME_ASSISTANT_URL + HOMELOOM_SECRET_KEY für Szenen und Licht
                       # optional: VAPID_PUBLIC_KEY + VAPID_PRIVATE_KEY für Mitteilungen (siehe .env.example)
                       # optional: TIME_ZONE für Erinnerungen und Kalender (Standard: Europe/Berlin)
                       # optional: OIDC_CLIENT_IDS, damit ein MCP-Client wie Claude PUBLIC_URL/mcp nutzen kann
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
open http://localhost:3000
```

## Kalender und Erinnerungen auf dem iPhone

Termine und Erinnerungen aus Homeloom gleichen sich über CalDAV in beide Richtungen mit den iPhone-Apps Kalender und Erinnerungen ab. Öffne Homeloom auf dem iPhone, geh zu Einstellungen › Andere Apps › Kalender und tipp auf „Zum iPhone hinzufügen“. Installiere dann das geladene Profil in den Einstellungen (iOS nennt es „Nicht überprüft“, weil es nicht signiert ist). Oder erstelle stattdessen ein Passwort und trag es unter Einstellungen › Apps › Kalender › Kalenderaccounts › Account hinzufügen › Andere › CalDAV-Account hinzufügen ein, mit deiner Homeloom-Adresse als Server und einem beliebigen Benutzernamen. Jede App zeigt eine gemeinsame Liste und eine nur für dich. Das Passwort öffnet nur deinen Kalender und deine Erinnerungen und gilt nicht mehr, sobald du es entfernst.

## Notizen in anderen Apps

Die Notizen aus Homeloom gibt es auch als Markdown-Dateien über WebDAV. So kann eine Notizen-App oder ein Dateimanager, der einen WebDAV-Ordner synchronisiert, sie lesen und ändern (Apple Notizen kann das nicht). Erstelle unter Einstellungen › Andere Apps › Notizen ein Passwort und füg in der App einen WebDAV-Account mit der angezeigten Adresse, einem beliebigen Benutzernamen und dem Passwort hinzu. Du bekommst einen Ordner „Shared“ und einen Ordner „Private“ nur für dich; Notizen für später bleiben draußen. Verschiebst du eine Datei in den anderen Ordner, teilst du sie oder machst sie privat.

## Roadmap

Homeloom wächst mit eurem gemeinsamen Alltag. Die Funktionen sind danach sortiert, was ihr macht, nicht nach Technik.

### Als Nächstes

**Rund ums Zuhause**
- **Fast zu Hause:** ein sanfter Hinweis, wenn jemand von euch auf dem Heimweg ist („Ofen schon an? Heute Abend: ...“).
- **Wochenrückblick am Sonntag:** eine Seite, die die Woche zusammenfasst.

**Rund um den Tisch**
- **Geschmacksprofile:** bewertet Gerichte und bekommt Ideen, die euch beiden schmecken.
- **Essenserinnerungen:** Fotos und Notizen zu euren Gerichten, dazu ein Jahresrückblick.

**Rund um euch zwei**
- **Briefe an die Zukunft:** schreibt euch, versiegelt bis zu einem Datum oder Meilenstein.
- **Dankbarkeitswand:** eine Zeile am Tag, die zufällig wieder auftaucht.
- **„Wir“-Zeitstrahl:** eure gemeinsamen Momente, mit Monatsrückblick und „Heute vor …“.

**Ausgehen und Daheimbleiben**
- **Date-Night-Ideen:** Vorschläge nach Wetter, Veranstaltungen in der Nähe, Budget und Laune.
- **Film- und Spieleabend:** ihr wischt beide, die Schnittmenge gewinnt, der Raum stellt sich passend ein.

### Später

- **Traumboard:** gemeinsame Ziele in kleinen Schritten, mit einem Check-in pro Quartal.
- **Perspektivenhilfe:** ihr schreibt beide eure Sicht privat und bekommt eine neutrale Zusammenfassung mit Kompromissideen. Zeigt nie den Text des anderen. Pro Eintrag freiwillig.
- **Ambient-Anzeige:** die Dankbarkeitszeile des Tages auf einem E-Ink-Display oder einem smarten Licht.
- **Haushaltsassistent:** stell deinem Zuhause eine Frage in ganz normaler Sprache.

### Ideen

Noch nicht sortiert und nicht versprochen.

**Rund ums Zuhause**
- **Haushaltsplan:** gemeinsame Aufgaben, die sich fair abwechseln.
- **Ankommen-Szenen:** Licht und Musik, wie ihr sie mögt, wenn ihr heimkommt, zusammen mit „Fast zu Hause“.
- **Pflanzen- und Hauspflege:** Gießen, Filter, Wartung, zur richtigen Zeit erinnert.
- **Energie im Blick:** ein ruhiger Blick darauf, was euer Zuhause diese Woche verbraucht hat.
- **Wunschliste fürs Zuhause:** Dinge, die ihr euch fürs Zuhause wünscht, gemeinsam gesammelt und sortiert.
- **Ruhezeiten:** Nicht stören für euch beide, und das Zuhause bleibt auch leise.
- **Ansicht für Gäste und Sitter:** eine kleine, zeitlich begrenzte Ansicht für Gäste oder Haussitter.

**Rund um den Tisch**
- **Was ist noch da:** Ideen aus dem Vorrat, was zuerst abläuft, kommt zuerst dran.

**Rund um euch zwei**
- **Wöchentlicher Check-in:** ein kurzes Ritual mit ein paar Fragen, die ihr zusammen beantwortet.
- **Jahrestage und Meilensteine:** die Daten, die zählen, nie mehr vergessen.
- **Geschenkideen:** private Ideen für deinen Schatz notieren, die vor Geburtstagen wieder auftauchen.
- **Sprachnachrichten:** hinterlasst euch kurze Nachrichten.
- **Gemeinsame Orte:** speichert die Orte, an denen ihr zusammen wart.

**Alltag**
- **Heute-Überblick:** eine Mitteilung am Morgen mit Wetter, Plänen und dem, was ansteht.
- **Budget im Blick:** ein einfacher Überblick über gemeinsame Ausgaben.
- **Reisen und Packlisten:** Reisen planen und mit gemeinsamen Listen packen.
- **Dokumentenablage:** wichtige Unterlagen an einem privaten Ort.
- **Merkliste:** alles, was ihr zusammen schauen, lesen oder besuchen wollt.

## An der API entwickeln

```sh
cd apps/api
go generate ./...   # nach Änderungen an openapi.yaml oder SQL
go test ./...       # mit TEST_DATABASE_URL und TEST_REDIS_URL laufen auch die Adapter-Tests
```

## An der App entwickeln

```sh
cd apps/web
npm ci
npm run dev    # http://localhost:5173, API-Aufrufe gehen an localhost:8080
npm test
```

## Releases

Versionen folgen SemVer und werden automatisch aus PR-Titeln im Conventional-Commits-Format veröffentlicht (`feat: …`, `fix: …`). Siehe `CHANGELOG.md` und die Images unter `ghcr.io/mariusgassen/homeloom-api`, `ghcr.io/mariusgassen/homeloom-web` und `ghcr.io/mariusgassen/homeloom-agent`.
