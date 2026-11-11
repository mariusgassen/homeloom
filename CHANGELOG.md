# Changelog

## [1.2.0](https://github.com/mariusgassen/homeloom/compare/homeloom-v1.1.0...homeloom-v1.2.0) (2026-11-11)


### Features

* store an optional birth year and allow years before 2000 ([#104](https://github.com/mariusgassen/homeloom/issues/104)) ([d34279c](https://github.com/mariusgassen/homeloom/commit/d34279c122be2d06f44a5aff3c9550164552259d))
* switch push notifications off per topic ([#99](https://github.com/mariusgassen/homeloom/issues/99)) ([29582e5](https://github.com/mariusgassen/homeloom/commit/29582e52f012cbc30c7ac122df629a0c57971f84))
* take the weather from the device's location ([#103](https://github.com/mariusgassen/homeloom/issues/103)) ([0fd8ac0](https://github.com/mariusgassen/homeloom/commit/0fd8ac04af0cff0b07d0cf4e4d7e1b065121158a))
* **web:** split Today into Us and Household tabs ([#110](https://github.com/mariusgassen/homeloom/issues/110)) ([36427bb](https://github.com/mariusgassen/homeloom/commit/36427bb24dcbff394d5a399895722a19f6a07695))


### Bug Fixes

* **deps-dev:** bump vitest from 5.0.5 to 5.1.0 in /apps/agent in the npm group ([#105](https://github.com/mariusgassen/homeloom/issues/105)) ([bcb99f0](https://github.com/mariusgassen/homeloom/commit/bcb99f0cac957104765bdaa61bd0f1e2408f7a81))
* **deps:** bump Go to 1.27.4 for GO-2026-6859 ([#109](https://github.com/mariusgassen/homeloom/issues/109)) ([a412c61](https://github.com/mariusgassen/homeloom/commit/a412c614b0a5b2e0cbbbea5a50312c74ae25e573))
* **deps:** bump the go-modules group in /apps/api with 4 updates ([#106](https://github.com/mariusgassen/homeloom/issues/106)) ([ba50fdb](https://github.com/mariusgassen/homeloom/commit/ba50fdbc2f5008bfe23ab57a7d194afc2f235ac3))
* **deps:** bump the npm group in /apps/web with 8 updates ([#107](https://github.com/mariusgassen/homeloom/issues/107)) ([8297f9b](https://github.com/mariusgassen/homeloom/commit/8297f9b28d17273f49fcbbba1e21be64e403d305))
* group the language import with third-party imports ([#102](https://github.com/mariusgassen/homeloom/issues/102)) ([73641d8](https://github.com/mariusgassen/homeloom/commit/73641d8f19e8d3ea8af820cd20b06b13eb8c7236))
* keep chores on Today and expose chores and wishes in MCP ([#108](https://github.com/mariusgassen/homeloom/issues/108)) ([f760cf2](https://github.com/mariusgassen/homeloom/commit/f760cf27fe0b7b16c1043baa7386b5f67c4ee3fa))
* show the place and a description in the forecast ([#100](https://github.com/mariusgassen/homeloom/issues/100)) ([4a80371](https://github.com/mariusgassen/homeloom/commit/4a8037160e5e2ab80feddd5ffed10bb97525dbb0))

## [1.1.0](https://github.com/mariusgassen/homeloom/compare/homeloom-v1.0.0...homeloom-v1.1.0) (2026-11-05)


### Features

* add a private documents vault ([#91](https://github.com/mariusgassen/homeloom/issues/91)) ([5e08af6](https://github.com/mariusgassen/homeloom/commit/5e08af67daac6e72f1bdd73db255573b3c038378))
* add a shared someday list of things to watch, read and visit ([#86](https://github.com/mariusgassen/homeloom/issues/86)) ([6ff56c9](https://github.com/mariusgassen/homeloom/commit/6ff56c9f2113f5afcb8a3bd99d6599252e5479d2))
* date-night generator with weather, events, budget and mood ([#88](https://github.com/mariusgassen/homeloom/issues/88)) ([072c5a2](https://github.com/mariusgassen/homeloom/commit/072c5a212a09c1cf813389610df54c4f845c5935))
* house wishlist ranked by hearts, with photos ([#92](https://github.com/mariusgassen/homeloom/issues/92)) ([91d31b9](https://github.com/mariusgassen/homeloom/commit/91d31b90183bed0477af78165cfc6a2d55b3d038))
* let date-night ideas follow limits and name real places ([#97](https://github.com/mariusgassen/homeloom/issues/97)) ([5961e5a](https://github.com/mariusgassen/homeloom/commit/5961e5afb35a72b540c8be27df5777f9b55ef534))
* morning briefing with weather, plans and what's due ([#84](https://github.com/mariusgassen/homeloom/issues/84)) ([d3d345c](https://github.com/mariusgassen/homeloom/commit/d3d345c5b4c94ca2f488809db91f78bf83755c87))
* movie night picker with swipes, matches and scene ([#87](https://github.com/mariusgassen/homeloom/issues/87)) ([dc04d93](https://github.com/mariusgassen/homeloom/commit/dc04d93cedd9d50be84706842f6b9b9009dce1c3))
* plan trips together with places, photos, favourites and a packing list ([#93](https://github.com/mariusgassen/homeloom/issues/93)) ([e063169](https://github.com/mariusgassen/homeloom/commit/e063169e5586d6420f09866eba799417ec3f7834))
* private gift ideas with birthday nudges ([#89](https://github.com/mariusgassen/homeloom/issues/89)) ([9cd7336](https://github.com/mariusgassen/homeloom/commit/9cd733623c37b2504d951fe496c1bcf1617371cb))
* react to moments with an emoji ([#85](https://github.com/mariusgassen/homeloom/issues/85)) ([1305688](https://github.com/mariusgassen/homeloom/commit/1305688263f04590fd4d3d293691fc5571534940))
* taste profiles and meal memories ([#90](https://github.com/mariusgassen/homeloom/issues/90)) ([64e8150](https://github.com/mariusgassen/homeloom/commit/64e8150a7f3c2b7d33b8d8a1eb5752132f8df7b3))


### Bug Fixes

* let mood and place lead date-night ideas, weather only filters ([#96](https://github.com/mariusgassen/homeloom/issues/96)) ([e6d2b85](https://github.com/mariusgassen/homeloom/commit/e6d2b855bfd17e6fd5b8d21e809be81319d35aaa))
* read date-night ideas after web search, and suggest places while typing ([#95](https://github.com/mariusgassen/homeloom/issues/95)) ([d416bda](https://github.com/mariusgassen/homeloom/commit/d416bdae762f92ebb31f387554e22261b5a31fec))
* **web:** mute the video in the commit, not after it ([#98](https://github.com/mariusgassen/homeloom/issues/98)) ([cd4f3eb](https://github.com/mariusgassen/homeloom/commit/cd4f3ebf78ae7ec192edebda517d8176a3c3ea79))

## 1.0.0 (2026-11-04)


### ⚠ BREAKING CHANGES

* MEALIE_API_TOKEN is gone. Set MEALIE_MCP_URL and MEALIE_MCP_TOKEN; MEALIE_URL is now optional and only used for links.

### Features

* a guest link works for the first device that uses it ([10cebf0](https://github.com/mariusgassen/homeloom/commit/10cebf0efdd48ec31295c5ae8223f6991ffde9de))
* add Garage S3 storage for moment photos ([65eab5c](https://github.com/mariusgassen/homeloom/commit/65eab5c8967863cf77f165925de78f0a619c25a4))
* add members and guests to a call ([65b1d31](https://github.com/mariusgassen/homeloom/commit/65b1d31a05d0729fe0e06583fcce213a0a9698b9))
* add the CalDAV account to an iPhone with a configuration profile ([f21eb7a](https://github.com/mariusgassen/homeloom/commit/f21eb7ab544da0158a5cc1ed94a2743b80a9622e))
* add the Homeloom logo and app icons ([9c0b197](https://github.com/mariusgassen/homeloom/commit/9c0b1977cda654fdf911cca630261bed6b542092))
* add the PWA shell with sign-in, tabs and German/English ([b47c41b](https://github.com/mariusgassen/homeloom/commit/b47c41bae75f90b1fa82136651ad56463d48f150))
* add the Us timeline with moments, gratitude lines and look-backs ([cb91e97](https://github.com/mariusgassen/homeloom/commit/cb91e978f3d94fe2e3f2dfeaf4a398d5d22b2325))
* **agent:** DTLN noise cancellation for the caller's audio ([65c3792](https://github.com/mariusgassen/homeloom/commit/65c3792a038b84dad4de775419968b57b574f8c9))
* **api:** AI question suggestions, edit/delete questions, AI watch ideas ([7112994](https://github.com/mariusgassen/homeloom/commit/7112994870df61c2593e31ba6557e640e1d5dfec))
* **api:** let members set their own profile picture ([7b0a0de](https://github.com/mariusgassen/homeloom/commit/7b0a0def6192d983b9b058cf63460a58227a7dd3))
* **api:** let the assistant script read-only tools (code mode) ([21bf26e](https://github.com/mariusgassen/homeloom/commit/21bf26edf493d3b95e0185efd5990d664a81c165))
* **api:** let the assistant script read-only tools (code mode) ([#74](https://github.com/mariusgassen/homeloom/issues/74)) ([3481203](https://github.com/mariusgassen/homeloom/commit/3481203a1c7a6ea8d325aae7b3ab68357b54944e))
* **api:** note titles from Markdown and photos in notes ([7444820](https://github.com/mariusgassen/homeloom/commit/7444820845686685ea0aba32b70da5d9e5d9c89a))
* background blur and face masks in calls ([035eafc](https://github.com/mariusgassen/homeloom/commit/035eafc41b30b48d1c8f033bd4ecbeaa7c3cce25))
* call one member of the household at a time ([d39b8b0](https://github.com/mariusgassen/homeloom/commit/d39b8b0a70eb5f19bd8b9f7a453e40a26882713f))
* chat assistant ([13ac9fd](https://github.com/mariusgassen/homeloom/commit/13ac9fd4e3314df416a589709e6be814a2007b83))
* chores page and today's chores on Today ([41f8e9b](https://github.com/mariusgassen/homeloom/commit/41f8e9b1213cd95d7662f7632a8c9b8b74af4986))
* chores with turns, points, history, fairness, nudges and thanks (API) ([eda51a1](https://github.com/mariusgassen/homeloom/commit/eda51a1b7c5caf5c8729a69a9079dce42a38ebd6))
* compress videos in the browser before they are sent ([1f278aa](https://github.com/mariusgassen/homeloom/commit/1f278aae5aae286190b72dc3ddc8b6df4cd41f70))
* cook by mood with Mealie and Claude ([1cf4517](https://github.com/mariusgassen/homeloom/commit/1cf4517b7f6c30fa00d36438572c7703d1e7865b))
* dinner mode and scenes with Home Assistant ([935e63d](https://github.com/mariusgassen/homeloom/commit/935e63d1177688d36d4ec4e28ec454e4d0a1e7a4))
* each person connects their own Home Assistant account ([52235a0](https://github.com/mariusgassen/homeloom/commit/52235a001c47e962cd2e8055a417c054358a859e))
* end-to-end encrypt every call ([06fa7b1](https://github.com/mariusgassen/homeloom/commit/06fa7b16d6da283096f8536e0b26d2d33c4910f1))
* feed-style video player for moments ([39490e6](https://github.com/mariusgassen/homeloom/commit/39490e6669f464debfffd69325624e196b9e8e1d))
* fullscreen for moment videos ([ec9fb91](https://github.com/mariusgassen/homeloom/commit/ec9fb911fe040fa46273e5f2691f829802b830ad))
* group the Home tab by room with live readings ([67b3df3](https://github.com/mariusgassen/homeloom/commit/67b3df362a90a2d8dd57b4d1f9e3ae5bf724a05f))
* hide done items and suggest titles used before ([ba5876f](https://github.com/mariusgassen/homeloom/commit/ba5876f6f8a053c89dcb484bb0c20aceb2c28a8b))
* invite your partner and answer questions together ([a6a7498](https://github.com/mariusgassen/homeloom/commit/a6a7498a48cf03da8bcb68c42566539b8c72a059))
* MCP server with the assistant's tools ([cff7aaa](https://github.com/mariusgassen/homeloom/commit/cff7aaa39a012fbc869db3926da7838729da6123))
* monthly spending by category and receipt photos ([19e9f76](https://github.com/mariusgassen/homeloom/commit/19e9f764254fe2381c402bc93b426b6f98e83520))
* movable call tiles, camera flip and emoji reactions ([3e0c19f](https://github.com/mariusgassen/homeloom/commit/3e0c19fcc67f96851868059fee257c1324648741))
* movies and shows from each person's Plex libraries ([d5cc96b](https://github.com/mariusgassen/homeloom/commit/d5cc96bc166e8890feb6c33960a69f5be3356fce))
* note preview, discard and delete, swipe actions on lists ([5eabcd8](https://github.com/mariusgassen/homeloom/commit/5eabcd880734a598398e2b74f4a4ab7cc69c3497))
* personal keys for other apps at /mcp ([90ded0a](https://github.com/mariusgassen/homeloom/commit/90ded0a4e0979fc027c7c0186bcd07ce089343c3))
* pick tonight's movie together and open it in Plex ([ba7a4f2](https://github.com/mariusgassen/homeloom/commit/ba7a4f257b2c5e0052accae953d4ae90cc7d525a))
* plan the week's dinners with recipes from Mealie ([a996617](https://github.com/mariusgassen/homeloom/commit/a9966170e899445e60e705a95eddfe07141954c2))
* plan the week's dinners with recipes from Mealie ([ac3b863](https://github.com/mariusgassen/homeloom/commit/ac3b86390e345eb5dbb261154633d0d7a7515ecc))
* pull to refresh and a branded, livelier app ([faa2ee4](https://github.com/mariusgassen/homeloom/commit/faa2ee4e05b48531667380bd46e38e6d47f22866))
* queue changes made offline and keep conflicting edits as copies ([f625318](https://github.com/mariusgassen/homeloom/commit/f625318f961b9c19aa494bfeffc2f18f97a0adad))
* reach Mealie through its MCP server ([119e02a](https://github.com/mariusgassen/homeloom/commit/119e02a377d735762d0f6ed68aaf96a99053823a))
* recipe pictures through the Mealie MCP server ([3a76686](https://github.com/mariusgassen/homeloom/commit/3a766869e6397e0089bde0f1ea25c0ab4cc12d88))
* recurring bills on reminders, next bill on Today, money tools for the assistant ([f9b9ac7](https://github.com/mariusgassen/homeloom/commit/f9b9ac784dfba833769c2880deb1b603d9671851))
* render markdown in assistant answers ([b79b527](https://github.com/mariusgassen/homeloom/commit/b79b5279cb1d70189dfebcbbd7c7e5a41539bf4c))
* shared calendar, reminders and notes ([ffb5d34](https://github.com/mariusgassen/homeloom/commit/ffb5d345b54a3e19112bac13cbff7024e9bb0a45))
* shared money with expenses, splits, balance and settling up ([b21e3c5](https://github.com/mariusgassen/homeloom/commit/b21e3c555633a1a0f51270f04ef0515feba2dac8))
* shared shopping list in Homeloom instead of Mealie's ([2b90c75](https://github.com/mariusgassen/homeloom/commit/2b90c753d1d3d5081749e16b11684682c643efd9))
* show Homeloom as an orb in calls and let each person pick its voice ([99c39dd](https://github.com/mariusgassen/homeloom/commit/99c39dd177a0b375b34bcb8a71850be82ab4261a))
* sign in through authentik with a server-side session ([8c3f6f9](https://github.com/mariusgassen/homeloom/commit/8c3f6f9d57bdf80d13664b6b2a5d104b808001c3))
* sort the shopping list into aisles ([e41e12b](https://github.com/mariusgassen/homeloom/commit/e41e12bdd94f184d8806aaee55caca466c6f3955))
* swipe right for done, left for delete ([a35a181](https://github.com/mariusgassen/homeloom/commit/a35a181977bfd0b1b513d135f783c1455ab41ea5))
* sync events and reminders with iPhone Calendar and Reminders over CalDAV ([76baceb](https://github.com/mariusgassen/homeloom/commit/76baceb7499c7d0af4060560269bdf2ec4b5116f))
* sync notes as Markdown files over WebDAV ([94d3289](https://github.com/mariusgassen/homeloom/commit/94d3289a8beeed5e94717187deae55d29874b4cf))
* take photos during calls ([8829c2c](https://github.com/mariusgassen/homeloom/commit/8829c2c8afdf164eaedc544d76e460bd11357207))
* talk to Homeloom in a voice call ([3ba5b56](https://github.com/mariusgassen/homeloom/commit/3ba5b56458cf3d76dd7d7d40a26250b2b7d0013c))
* use the final Homeloom icons in the PWA ([1e816fb](https://github.com/mariusgassen/homeloom/commit/1e816fb5bf6593a4b187117b7a2f1a2c1f8b8f23))
* videos on moments, with autoplay on or off ([d03557c](https://github.com/mariusgassen/homeloom/commit/d03557c4eef6b483ad8603b781edb130669dc769))
* voice and video calls between household members ([81b6f65](https://github.com/mariusgassen/homeloom/commit/81b6f65060055f8f5f3e1a682d5d6b4397ce62f8))
* Web Push notifications with VAPID ([f2588b5](https://github.com/mariusgassen/homeloom/commit/f2588b51b9e3f6b668205318bc47a58e72bd0f82))
* **web:** context-aware add button above the tab bar ([ae80a70](https://github.com/mariusgassen/homeloom/commit/ae80a708ae73226abacbb212a1efc8944911a207))
* **web:** drop the room name from device names in Home ([#83](https://github.com/mariusgassen/homeloom/issues/83)) ([d658aee](https://github.com/mariusgassen/homeloom/commit/d658aee4842d8031671a18e5c1124aae85343bb3))
* **web:** four tabs with dinner and movies on Today; shopping by aisle ([45d6306](https://github.com/mariusgassen/homeloom/commit/45d6306ae3d32717a27f23e4f8abc11f12ff2f26))
* **web:** fresh question suggestions, edit/delete questions, watch ideas ([78a3796](https://github.com/mariusgassen/homeloom/commit/78a3796dc08c8e025b043dc58ed3ff3e078cb798))
* **web:** keep expenses and chores offline too ([fac6ad7](https://github.com/mariusgassen/homeloom/commit/fac6ad72762cde9e3dfa0414992468e1983f55fa))
* **web:** keyboard-aware sheets and tap controls instead of dropdowns ([86f8159](https://github.com/mariusgassen/homeloom/commit/86f8159b9f8fbce6e7d735594db3af9dadd6974c))
* **web:** Markdown notes with a title and photos ([bf2f04d](https://github.com/mariusgassen/homeloom/commit/bf2f04d82aab19df2e864ee27e2a1b1630771198))
* **web:** open instantly and read offline from what the device kept ([56f2561](https://github.com/mariusgassen/homeloom/commit/56f25618270dd1dfd48b0a574e0e18581842e7dd))
* **web:** open notes as their own page that saves as you write ([5973082](https://github.com/mariusgassen/homeloom/commit/5973082042ef7c2bf615e937097817ff00d133f6))
* **web:** pick your own profile picture in Settings ([75483c3](https://github.com/mariusgassen/homeloom/commit/75483c3798ece7aadfc9578db5bf03d3e4076902))
* **web:** show when changes wait, go out or were kept as copies ([72ef313](https://github.com/mariusgassen/homeloom/commit/72ef313cbb9a3c69ee90026c1e01aa8cbb14d5f9))
* **web:** warmer Today and livelier Home panels ([a432d3c](https://github.com/mariusgassen/homeloom/commit/a432d3cf0dd305ea647de5aaa26d40cedcce5b94))


### Bug Fixes

* **agent:** share Silero's onnxruntime-node ([d4a6243](https://github.com/mariusgassen/homeloom/commit/d4a624302999f452ef6bc2935f466bbfa9c2158e))
* **agent:** tolerate an empty LOG_LEVEL ([a25f754](https://github.com/mariusgassen/homeloom/commit/a25f7544bbea5ad45093b9e1338986488faf482a))
* answer CalDAV discovery the way iOS expects ([75bdaa0](https://github.com/mariusgassen/homeloom/commit/75bdaa0fc4908e1cbc283c6625c3ce969bc6d52a))
* **api:** build recipe tags without a summed allocation size ([71a32df](https://github.com/mariusgassen/homeloom/commit/71a32df9ca68c0397f37face643cc1496deca6f7))
* clear adm-zip advisories in the voice agent, skip MediaPipe's unused module build ([a8d9e6f](https://github.com/mariusgassen/homeloom/commit/a8d9e6fdf48df6222ba7aef9b5e5bf92090e7599))
* **deps:** bump @mediapipe/tasks-vision ([28f9bca](https://github.com/mariusgassen/homeloom/commit/28f9bcabe312aeba8536cb21de86ed1ca310751e))
* **deps:** bump the go-modules group in /apps/api with 2 updates ([8f15109](https://github.com/mariusgassen/homeloom/commit/8f15109b4d0c58e7b19b0dbf960febb1317229c7))
* **deps:** bump the go-modules group in /apps/api with 2 updates ([#71](https://github.com/mariusgassen/homeloom/issues/71)) ([e42d125](https://github.com/mariusgassen/homeloom/commit/e42d1250b4379832e68d3edef38364b47a62bea8))
* **deps:** bump the npm group in /apps/agent with 7 updates ([#78](https://github.com/mariusgassen/homeloom/issues/78)) ([63b5328](https://github.com/mariusgassen/homeloom/commit/63b53289f596c23eeb2cecfae460fb51bc13ef4b))
* **deps:** bump the npm group in /apps/web with 7 updates ([#79](https://github.com/mariusgassen/homeloom/issues/79)) ([7a2b56a](https://github.com/mariusgassen/homeloom/commit/7a2b56ae2108b05b00fddadc672c9b66a48217b1))
* drop the ${VAR:+…} compose interpolation Coolify can't parse ([9bb80e6](https://github.com/mariusgassen/homeloom/commit/9bb80e6efc3a7d4b82380ee759c025d34a511c7d))
* give homeloom-livekit a health check ([a38c1be](https://github.com/mariusgassen/homeloom/commit/a38c1beade865c9747f8519b73d3286ff8756d59))
* keep calendar passwords as bcrypt hashes ([d579244](https://github.com/mariusgassen/homeloom/commit/d57924436da47a88f0871fd945b3ab0df79701f1))
* let WebDAV and CalDAV methods through the router ([01744c7](https://github.com/mariusgassen/homeloom/commit/01744c745512306c344b264b252648a0fb0a2b31))
* open movies when tapped, filter them, choose which libraries count ([2dc0205](https://github.com/mariusgassen/homeloom/commit/2dc02052fc71329dd7115d0644f7dc3f71b941ac))
* run required checks on the release PR ([#82](https://github.com/mariusgassen/homeloom/issues/82)) ([40e9757](https://github.com/mariusgassen/homeloom/commit/40e9757da3196541c31a03c7ea28b3d47c49019b))
* satisfy golangci-lint in the Plex adapter ([c9926fe](https://github.com/mariusgassen/homeloom/commit/c9926fe526286542ecf6f50aaac96f6d04f1e20f))
* solid status bar, shorter copy, recipe times from Mealie's seconds ([c0e90a8](https://github.com/mariusgassen/homeloom/commit/c0e90a8d744574b74379037433353db6a834a5d3))
* take HOMELOOM_SECRET_KEY as a random 32-byte key instead of hashing a string ([241cd09](https://github.com/mariusgassen/homeloom/commit/241cd09ebe36217aac38679a561a0f5d51112587))
* use a tagged switch for the collection PROPFIND ([f868424](https://github.com/mariusgassen/homeloom/commit/f86842495a77af6de57706f42da1eb52303f0b52))
* **web:** don't show another tab's entries while a list loads ([99ff491](https://github.com/mariusgassen/homeloom/commit/99ff491f6e1271b26fb097df1061d66ca0ec10a0))
* **web:** filled add button so it doesn't look like the assistant's ([971d655](https://github.com/mariusgassen/homeloom/commit/971d655fac21bcc7645d0c314d92ed711f4aa4e5))
* **web:** open the right screen from a notification and answer calls from it ([2930f6f](https://github.com/mariusgassen/homeloom/commit/2930f6fb598b0aa7295b57b19b8a9338a5fe91fd))
* **web:** page-coloured strip behind the iOS status bar ([07e9830](https://github.com/mariusgassen/homeloom/commit/07e98302d02ad8a0052010391ca584ab22e4b77e))
* **web:** readable sheets in dark mode ([18f84eb](https://github.com/mariusgassen/homeloom/commit/18f84ebfd2d37ed0d9c861d0544638d19a82b86b))
* **web:** solid top bar under the iOS clock and Lists tabs that fit ([8a1251a](https://github.com/mariusgassen/homeloom/commit/8a1251a13e1ff27f56a5dd2dee3ccd55639e8533))
