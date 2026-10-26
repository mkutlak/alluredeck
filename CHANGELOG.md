## [0.51.0](https://github.com/mkutlak/alluredeck/compare/v0.50.0...v0.51.0) (2026-10-26)

### Features

* **e2e:** seed demo data and regenerate doc screenshots ([2d7e22d](https://github.com/mkutlak/alluredeck/commit/2d7e22dada9b39bd42bc244a4d2aa05338cc9430))

### Bug Fixes

* **api:** record flaky tests from Allure 3 reports once per test ([5a28d01](https://github.com/mkutlak/alluredeck/commit/5a28d014860f34934c8a0736eb9b04adee9e746c))
* **api:** resolve the latest build deterministically across branches ([34599a0](https://github.com/mkutlak/alluredeck/commit/34599a024fecacc5942a76138ac9d1e1f9f758ba))
* **ui:** colour the Suite Pass Rates bars ([ba346de](https://github.com/mkutlak/alluredeck/commit/ba346de67ff9a0dec130a640578252c3db41e69e))
* **ui:** keep the Projects breadcrumb readable and link it to /projects ([d7794a7](https://github.com/mkutlak/alluredeck/commit/d7794a7a4cd96de0aa5c6d2dcabca8bd7aabb61a))
* **ui:** name projects by label in dialogs and the report viewer ([bf8368e](https://github.com/mkutlak/alluredeck/commit/bf8368eefa873aa8c6ee67c5d6431f2619f32be1))
* **ui:** read build_number after the API rename ([04b7bf9](https://github.com/mkutlak/alluredeck/commit/04b7bf957b6ce13345f3615365299e8f4d208d03))

## [0.50.0](https://github.com/mkutlak/alluredeck/compare/v0.49.1...v0.50.0) (2026-10-26)

### Features

* **ui:** give the Pipeline Runs card a verdict and state its pass rate as a fact ([8532c9d](https://github.com/mkutlak/alluredeck/commit/8532c9d20d2773c9021ae4cb4d60da14284f742d))

### Bug Fixes

* **ui:** give a suite whose report could not be read its chip in the feed ([a736629](https://github.com/mkutlak/alluredeck/commit/a73662984b50e19bd4755b2525103ba041a59d26))
* **ui:** key pipeline run cards by run, not by commit ([318db31](https://github.com/mkutlak/alluredeck/commit/318db312350909cd444511bc5fd36e0c6c954eae))
* **ui:** make command palette group headings readable ([067ddd5](https://github.com/mkutlak/alluredeck/commit/067ddd528bc76c93a5f6a727fa49ce19925e1f99))
* **ui:** say a newly failing suite's tests are loading, not missing ([6a16d3c](https://github.com/mkutlak/alluredeck/commit/6a16d3c48c434659a63c198ccdb232158aed85a9))
* **ui:** set skipped and neutral badge text in Graphite ([adaadd4](https://github.com/mkutlak/alluredeck/commit/adaadd4bf258d8db37725804b80ef9f6d2f817a5)), closes [#6c6f85](https://github.com/mkutlak/alluredeck/issues/6c6f85) [#5c5f77](https://github.com/mkutlak/alluredeck/issues/5c5f77)

## [0.49.1](https://github.com/mkutlak/alluredeck/compare/v0.49.0...v0.49.1) (2026-10-12)

### Bug Fixes

* **api:** ask MCP clients to sign in again when their key or account is refused ([2a24469](https://github.com/mkutlak/alluredeck/commit/2a244697d9f65ff59a348fe4f702d7e69f081bdc))
* **api:** check the users table before treating a key owner as an env user ([03f8287](https://github.com/mkutlak/alluredeck/commit/03f8287a2dc91e2788ef7797b8842464d71c9997))
* **api:** copy S3 objects whose keys need escaping ([6b42964](https://github.com/mkutlak/alluredeck/commit/6b4296464e82535ca2bda141880f79eeb54641e7))
* **api:** don't fail a user's other requests when one cancels its account check ([e2bc4b5](https://github.com/mkutlak/alluredeck/commit/e2bc4b52975c8cbde1c10c565d2c451330e7c2f8))
* **api:** don't send API-key holders to sign in again when the database fails ([b5b0675](https://github.com/mkutlak/alluredeck/commit/b5b06753dee2a0a6429108bff8e9a63cb82a8829))
* **api:** forget the env-user cache entry when an account is deactivated ([968f494](https://github.com/mkutlak/alluredeck/commit/968f4940d23f4e76be9d22b58698547ec2229630))
* **api:** keep webhooks readable when the JWT secret changes ([1ed2d80](https://github.com/mkutlak/alluredeck/commit/1ed2d80cced18da907ff6e03a6febabefd1ecd1a))
* **api:** let the metrics setup run more than once ([d7d5cd8](https://github.com/mkutlak/alluredeck/commit/d7d5cd88fa7749cc1f9f933bd342013b734949e5))
* **api:** notice deactivated users within seconds ([c4849af](https://github.com/mkutlak/alluredeck/commit/c4849af45328ea8f1dba5a3ee9b50bae4869d0d5))
* **api:** put trace IDs on request log lines ([e37e5fc](https://github.com/mkutlak/alluredeck/commit/e37e5fcea2729a9ce49a59485b64fdcb804a21f8))
* **api:** stop accepting API keys whose owner can't be found ([655b499](https://github.com/mkutlak/alluredeck/commit/655b49965efbd25053f0a5b97bc029b273b47dbb))
* **api:** stop marking numbered report builds immutable ([da4cdf4](https://github.com/mkutlak/alluredeck/commit/da4cdf487587b69da3806e4457e0d746c159c48f))
* **docker:** give the MCP server the env user names ([24761c0](https://github.com/mkutlak/alluredeck/commit/24761c047761531d1ead176a8bf68de78b86aa54))
* **helm:** give the MCP server the env user names ([91aa258](https://github.com/mkutlak/alluredeck/commit/91aa258723a200a7634318f98c3173e68278990e))
* **ui:** act on projects by numeric id, and refresh every query of one ([6bd1940](https://github.com/mkutlak/alluredeck/commit/6bd194080ca3f5aab9e6c99583804f9f06baf3c2))
* **ui:** always revalidate API responses with the server ([cf97dea](https://github.com/mkutlak/alluredeck/commit/cf97dea0b49bebf5e34724beb8c40780fa8ef60c))
* **ui:** catch renaming a child project to its current name again ([49b5744](https://github.com/mkutlak/alluredeck/commit/49b5744cc4f8488f360106474cac6173431516bc))
* **ui:** clean pending results of the project you clicked ([da35084](https://github.com/mkutlak/alluredeck/commit/da3508488adc738881869599d84a320c147d36cc))
* **ui:** name projects by their full path in project dialogs ([c25f934](https://github.com/mkutlak/alluredeck/commit/c25f9345f3feaf6afc65ab3bc51822a7f997c42d))

## [0.49.0](https://github.com/mkutlak/alluredeck/compare/v0.48.0...v0.49.0) (2026-10-12)

### Features

* **api:** count failures of unknown origin in the runs feed ([5d3b8ed](https://github.com/mkutlak/alluredeck/commit/5d3b8edfad637efc0003fdccc3cde6850a4851ce))
* **api:** find pipeline runs and branches in search ([9d2b0ce](https://github.com/mkutlak/alluredeck/commit/9d2b0ce7e2d1d70c09feb8eb443f68aea1f858f3))
* **api:** give each run failure a cluster key shared with the pipeline diagnosis ([4f42822](https://github.com/mkutlak/alluredeck/commit/4f42822729810f1cb4a7a37111feaeb5ef617048))
* **api:** name the deepest failed step of each run failure ([d845dba](https://github.com/mkutlak/alluredeck/commit/d845dba8a1339622aa84465834b0b63bfc70d560))
* **api:** say how many suites a pipeline run is still waiting for ([f470738](https://github.com/mkutlak/alluredeck/commit/f4707383b8679ed2a75de0e8a59a1ae0ef3de93d))
* **api:** say where each listed run's failures came from ([a921a98](https://github.com/mkutlak/alluredeck/commit/a921a989d291b813378e265d52df5d88e5ba1ba6))
* **api:** serve a single pipeline run and tag failures as new, inherited or environment ([6270273](https://github.com/mkutlak/alluredeck/commit/627027333e4ea15c5f5abac41b6357a5e04081fd))
* **ui:** add a page for each run and keep the runs feed state in the URL ([9f109d2](https://github.com/mkutlak/alluredeck/commit/9f109d2ffa14d8e149ef33b5f907e0d3000bae98))
* **ui:** find a pipeline run or a branch from the search palette ([df56661](https://github.com/mkutlak/alluredeck/commit/df56661d1e721757cb91930b74d1df23db8bf8a8))
* **ui:** group failures by the server's cluster key ([f6a7c93](https://github.com/mkutlak/alluredeck/commit/f6a7c93fc0276dcbd5c62472fd180eafc660591a))
* **ui:** name each run's verdict and the CI links for screen readers ([93cb26e](https://github.com/mkutlak/alluredeck/commit/93cb26e98b2bf986795dd83fb5dc94e489920e3f))
* **ui:** refresh a run that is still uploading until it is complete ([6fc3ca4](https://github.com/mkutlak/alluredeck/commit/6fc3ca40f9f833251f3d47a74141f057fa78a439))
* **ui:** say "none new" and stop hiding failures of unknown origin ([8fd2889](https://github.com/mkutlak/alluredeck/commit/8fd288991709a59b19143b4da9319c36605795c2))
* **ui:** show a run that is still uploading as awaiting suites, not as finished ([5375f42](https://github.com/mkutlak/alluredeck/commit/5375f422db3e49b53810675708c94b2f6ff48cd4))
* **ui:** show the failed step under each error group's heading ([e5913c9](https://github.com/mkutlak/alluredeck/commit/e5913c997ba52fa0307b9be6dd2bd262f2b3f971))
* **ui:** show where a run's failures came from without expanding it ([21f05f2](https://github.com/mkutlak/alluredeck/commit/21f05f210ab797c202a8b183db7fb45e3ef60525))
* **ui:** tag each failure as new, inherited or environment ([1405891](https://github.com/mkutlak/alluredeck/commit/1405891a41c4b4804c3bf5c17c472c18aac18510))

### Bug Fixes

* **api:** count environment failures as inherited in the pipeline diagnosis ([1a6a1d0](https://github.com/mkutlak/alluredeck/commit/1a6a1d083343ffdb057f715786c2653a991f6627))
* **api:** never cache the failures of a pipeline run ([6b43cea](https://github.com/mkutlak/alluredeck/commit/6b43ceafb1e2a874020c20bfd69d9c29a43a3a33))
* **api:** never cache the lists of pipeline runs ([d63c1e6](https://github.com/mkutlak/alluredeck/commit/d63c1e631647d0611cb6b72cfea1a5d4fabe9af2))
* **helm:** let helm test find the connection test pods ([6ff73ea](https://github.com/mkutlak/alluredeck/commit/6ff73ea24a13c9db46aee46ddfa110b61d0ba0cc))
* **ui:** apply the saved theme before the first paint ([165b67e](https://github.com/mkutlak/alluredeck/commit/165b67e391d304d27449f30a88496523b8b8eb88))
* **ui:** keep "none new" out of the failure panel when the row withholds it ([21bc8e5](https://github.com/mkutlak/alluredeck/commit/21bc8e5a20e290e22c82981ae77bb34a128324e1))
* **ui:** line up the failure panel's columns and move the AI control last ([14ab567](https://github.com/mkutlak/alluredeck/commit/14ab56746a0d934162ed24b64ca9ad3d8d02d1ac))
* **ui:** make the focus ring and the feed's badge text readable in light mode ([ed1d81d](https://github.com/mkutlak/alluredeck/commit/ed1d81ddeba8a7f3f87953e222ccfbb20d849d37)), closes [#6c6f85](https://github.com/mkutlak/alluredeck/issues/6c6f85) [#5c5f77](https://github.com/mkutlak/alluredeck/issues/5c5f77)
* **ui:** make the runs feed usable by keyboard and screen reader ([7ea5848](https://github.com/mkutlak/alluredeck/commit/7ea5848dcbcdf3dc274e2c89a974d14746d03d47))
* **ui:** reflow the runs feed rows and remember the sidebar state ([7001405](https://github.com/mkutlak/alluredeck/commit/7001405a9623beb0ef87796f2e6a65aab2c60b75))
* **ui:** show the focus ring on a chart that has keyboard focus ([c269bfb](https://github.com/mkutlak/alluredeck/commit/c269bfbcbbb07fabc20eaca5003c75e21a2f2031))
* **ui:** stop repeating the failure and suite counts in the failures panel ([a9ebf51](https://github.com/mkutlak/alluredeck/commit/a9ebf51af6e9d3cdc6008fae576ec3e4bd0602b8))
* **ui:** stop the theme provider's script tag from warning in development ([d216f83](https://github.com/mkutlak/alluredeck/commit/d216f83942660cb82bd15d6bf4bded45740afced))

## [0.48.0](https://github.com/mkutlak/alluredeck/compare/v0.47.3...v0.48.0) (2026-10-07)

### Features

* **ui:** make the runs feed readable and its pass rates honest ([5d56e6b](https://github.com/mkutlak/alluredeck/commit/5d56e6b3fe2bf0c27cfd07b5f71b6f0422133d17))

### Bug Fixes

* **api:** floor pipeline pass rates and report skipped suites ([5986d69](https://github.com/mkutlak/alluredeck/commit/5986d69e62bf4dee408f1d5c14b1b960f2fdb712))

## [0.47.3](https://github.com/mkutlak/alluredeck/compare/v0.47.2...v0.47.3) (2026-10-07)

### Bug Fixes

* **api:** count the API-key limit under the stored username ([4d2f3e9](https://github.com/mkutlak/alluredeck/commit/4d2f3e9d9834cc36f3aeb9dfa007aab5ce45140e))
* **api:** end a deactivated user's access on the next request ([8d268b7](https://github.com/mkutlak/alluredeck/commit/8d268b717d53ae5c4f217ab6f281b3e1804072d2))
* **api:** end OIDC sessions on deactivation ([0fe51ce](https://github.com/mkutlak/alluredeck/commit/0fe51ce60d812048d3370bb5e82757ae6ac93e00))
* **api:** refuse token refresh for deactivated users ([eddaa39](https://github.com/mkutlak/alluredeck/commit/eddaa39b6fd30afa402a033a532368596036882d))
* **storage:** report S3 Allure 3 build duration as wall clock ([8b776f7](https://github.com/mkutlak/alluredeck/commit/8b776f7deed347a2c933ffb513708a912c8cc441))
* **ui:** build compare and report links from the numeric project_id ([2f1ee3c](https://github.com/mkutlak/alluredeck/commit/2f1ee3caeaa960f984b7a83071deaca54157d952))
* **ui:** invalidate project caches by clean key prefixes ([80da082](https://github.com/mkutlak/alluredeck/commit/80da0825d955b1d53348a2adb23f66e08fbe5f63))
* **ui:** pass the effective branch to the analytics cards ([076d7f3](https://github.com/mkutlak/alluredeck/commit/076d7f3401e9f1f2106bdefa218f5f334715f1d6))
* **ui:** refresh the known-issues list after changes ([faf8b81](https://github.com/mkutlak/alluredeck/commit/faf8b812d3cda59b8afca0dbef3f9be0ddba7276))

## [0.47.2](https://github.com/mkutlak/alluredeck/compare/v0.47.1...v0.47.2) (2026-08-25)

### Bug Fixes

* **api:** batch stale-branch GC, prune orphan branch rows, use storage_key in retention sweep ([d086551](https://github.com/mkutlak/alluredeck/commit/d08655173374c3ce1f05fe82f2c12f4237a00315))

## [0.47.1](https://github.com/mkutlak/alluredeck/compare/v0.47.0...v0.47.1) (2026-08-25)

### Bug Fixes

* **store:** move shell-twin cleanup off the startup migration path ([c8c3a34](https://github.com/mkutlak/alluredeck/commit/c8c3a3432924121e77807e1111989a6670baa072))

## [0.47.0](https://github.com/mkutlak/alluredeck/compare/v0.46.0...v0.47.0) (2026-08-25)

### Features

* **api:** per-retry attempt persistence, branch-aware history, and triage signal accuracy ([385dbdd](https://github.com/mkutlak/alluredeck/commit/385dbdd9a34d567301631a8b975628dd08b6c805))
* **mcp:** clustering, build verdicts, attachment content, and pipeline-wide diagnosis ([075eb73](https://github.com/mkutlak/alluredeck/commit/075eb735ef22f07bda01a557a330820ee9a768ba))

### Bug Fixes

* **ci:** make the e2e stack writable and key project storage by storage_key ([7741a20](https://github.com/mkutlak/alluredeck/commit/7741a20e9d98897585f9f6674ce54e552ffdda3b))
* **ingest:** reconcile Allure dual-write history_id schemes and purge shell twins ([0501196](https://github.com/mkutlak/alluredeck/commit/05011965f0ae0dc9813fb078713998b84e50dae4))

## [0.46.0](https://github.com/mkutlak/alluredeck/compare/v0.45.0...v0.46.0) (2026-07-30)

## [0.45.0](https://github.com/mkutlak/alluredeck/compare/v0.44.1...v0.45.0) (2026-07-29)

## [0.44.1](https://github.com/mkutlak/alluredeck/compare/v0.44.0...v0.44.1) (2026-07-14)

## [0.44.0](https://github.com/mkutlak/alluredeck/compare/v0.43.0...v0.44.0) (2026-07-05)

## [0.43.0](https://github.com/mkutlak/alluredeck/compare/v0.42.0...v0.43.0) (2026-07-05)

## [0.42.0](https://github.com/mkutlak/alluredeck/compare/v0.41.1...v0.42.0) (2026-07-05)

## [0.41.1](https://github.com/mkutlak/alluredeck/compare/v0.41.0...v0.41.1) (2026-07-04)

## [0.41.0](https://github.com/mkutlak/alluredeck/compare/v0.40.2...v0.41.0) (2026-07-03)

## [0.40.2](https://github.com/mkutlak/alluredeck/compare/v0.40.1...v0.40.2) (2026-06-29)

## [0.40.1](https://github.com/mkutlak/alluredeck/compare/v0.40.0...v0.40.1) (2026-06-25)

### Bug Fixes

* **api:** collapse duplicate Allure historyIds when storing test results ([9a6773b](https://github.com/mkutlak/alluredeck/commit/9a6773be87b8a648233a3eb6d3bc6db5020ae316))

## [0.40.0](https://github.com/mkutlak/alluredeck/compare/v0.39.0...v0.40.0) (2026-06-25)

### Features

* **api,ui:** exclude skipped tests from pass-rate denominator ([74653e3](https://github.com/mkutlak/alluredeck/commit/74653e3e725ca354259fa8fa5f2e119c19468f76))

## [0.39.0](https://github.com/mkutlak/alluredeck/compare/v0.38.0...v0.39.0) (2026-06-10)

### Features

* **api,chart:** serialize migrations with advisory lock + optional pre-upgrade Job ([da79076](https://github.com/mkutlak/alluredeck/commit/da7907673f5ab66574a132d4a856d9bd4f4015b2))
* **api,ui:** invert notifications + add job retry ([e1e1539](https://github.com/mkutlak/alluredeck/commit/e1e1539ee8947f3dbec7ba5af2f5f85982921225))
* **api:** DB timeout GUCs, panic stack traces, opt-in pprof, DSN validation ([8ace09b](https://github.com/mkutlak/alluredeck/commit/8ace09bd38f7f71d679528feb8e1acf0473cc076))
* **api:** readiness probe reports per-dependency health ([fabe16d](https://github.com/mkutlak/alluredeck/commit/fabe16dc53349fe4550b76f666700a3ad50aa74e))
* **api:** webhook SSRF delivery pinning + API-key project scope ([0ad1441](https://github.com/mkutlak/alluredeck/commit/0ad1441a2fe85835bba8de529e17d6c6dd47f9df))
* **chart:** add HPA, PDB, resource defaults, local-storage rollout safety ([a8a4b14](https://github.com/mkutlak/alluredeck/commit/a8a4b1482e70a671cb0bd60b013549c7e918d9b0))
* **ui:** surface backend degradation instead of hiding it ([2f8ab88](https://github.com/mkutlak/alluredeck/commit/2f8ab887aacc34bb3b72b4f16bfe8157a3134301))

### Performance Improvements

* **api:** batch job-progress, set-based pruning, analytics indexes, bounded S3 reads ([54a1cde](https://github.com/mkutlak/alluredeck/commit/54a1cde5f80cf6cbe39218de75a7c393a8d8eb73))

## [0.38.0](https://github.com/mkutlak/alluredeck/compare/v0.37.3...v0.38.0) (2026-05-22)

### Features

* **api:** expose Allure environment.properties via MCP ([4c73eef](https://github.com/mkutlak/alluredeck/commit/4c73eef45d5ffae4c3e5438435178d9f6998ffeb))

## [0.37.3](https://github.com/mkutlak/alluredeck/compare/v0.37.2...v0.37.3) (2026-05-21)

### Bug Fixes

* **api:** persist job report_id durably via River output ([c0517a7](https://github.com/mkutlak/alluredeck/commit/c0517a7246ed2bd6165f0b9b1590016f8445cfa8))

## [0.37.2](https://github.com/mkutlak/alluredeck/compare/v0.37.1...v0.37.2) (2026-05-18)

### Bug Fixes

* **mcp:** wire external_url + storage so attachment resources resolve ([da9e447](https://github.com/mkutlak/alluredeck/commit/da9e447ab6dd0cf17a52efe9425c3c9adbbf202f))

## [0.37.1](https://github.com/mkutlak/alluredeck/compare/v0.37.0...v0.37.1) (2026-05-18)

### Bug Fixes

* **api:** use builds.build_order in attachment location query ([402075c](https://github.com/mkutlak/alluredeck/commit/402075c3f1b101a7389af69abf20dc261df20efe))

## [0.37.0](https://github.com/mkutlak/alluredeck/compare/v0.36.0...v0.37.0) (2026-05-18)

### Features

* **ui:** relocate branch selector to shared breadcrumb bar ([412674f](https://github.com/mkutlak/alluredeck/commit/412674fbfd36e1f1ac2aa1494540c2a85ada066c))

## [0.36.0](https://github.com/mkutlak/alluredeck/compare/v0.35.1...v0.36.0) (2026-05-17)

### Features

* **mcp:** add diagnose_failure tool and decompose store interfaces ([653c91e](https://github.com/mkutlak/alluredeck/commit/653c91e401ed9106835c7194ac40d1bad0c006f9))

## [0.35.1](https://github.com/mkutlak/alluredeck/compare/v0.35.0...v0.35.1) (2026-05-16)

### Bug Fixes

* **api:** resolve numeric JWT sub to email when listing/deleting API keys ([4cb6a7d](https://github.com/mkutlak/alluredeck/commit/4cb6a7dc58d45cd5fd7abf5829dab263abe347c0))

## [0.35.0](https://github.com/mkutlak/alluredeck/compare/v0.34.2...v0.35.0) (2026-05-15)

### Features

* **mcp:** add Model Context Protocol server with proposal-mode mutations ([97a5f55](https://github.com/mkutlak/alluredeck/commit/97a5f558d2a08f57891936fa01d94d3ef08e5d46))
* **mcp:** harden Kubernetes deployment and wire image into release pipeline ([8ae3ab2](https://github.com/mkutlak/alluredeck/commit/8ae3ab251c8f099ca6110f7ec7c5572d2dc83062))
* **mcp:** single-shot URL resolution, dedupe, and tighter validation ([07b1bb4](https://github.com/mkutlak/alluredeck/commit/07b1bb4989cb9448378528ab2064f3b9c8a9ea70))

### Bug Fixes

* **mcp:** populate TokenInfo.Expiration so auth no longer 401s ([1b80c3c](https://github.com/mkutlak/alluredeck/commit/1b80c3c9efc9a3547e45c0522ab638025e78bffa))

## [0.34.2](https://github.com/mkutlak/alluredeck/compare/v0.34.1...v0.34.2) (2026-05-10)

### Bug Fixes

* **ui:** never round pass rate up to 100% when there are failures ([bb96c92](https://github.com/mkutlak/alluredeck/commit/bb96c928e4cdcf8642058c017cb93c91fadaf229))

## [0.34.1](https://github.com/mkutlak/alluredeck/compare/v0.34.0...v0.34.1) (2026-05-10)

### Bug Fixes

* **jobs:** wire progress callbacks into preparing_local and publishing_report ([224ad48](https://github.com/mkutlak/alluredeck/commit/224ad48fd56b0dfc73356106fabacdd7e5877eff))

### Performance Improvements

* **staged-upload:** extract tar.gz to pod-local disk, skip MinIO round-trip ([8679b38](https://github.com/mkutlak/alluredeck/commit/8679b38e70b4dabd4449d67b81b3b988f7276ad2))

## [0.34.0](https://github.com/mkutlak/alluredeck/compare/v0.33.2...v0.34.0) (2026-05-09)

### Features

* **jobs:** expose phase and progress in /jobs/{id} ([a801d0d](https://github.com/mkutlak/alluredeck/commit/a801d0d2e850cdfc28d9b731c3d2a704fc560fc1))
* **upload:** async tar.gz upload via staging blob + River worker ([24fe265](https://github.com/mkutlak/alluredeck/commit/24fe26552db43947b3c9cfc30869b0296e0daa79))

### Performance Improvements

* **playwright:** spool tar entries to disk to bound peak heap ([584ab9b](https://github.com/mkutlak/alluredeck/commit/584ab9b84a27efd0f0a7ff62e2d452083741aa74))

## [0.33.2](https://github.com/mkutlak/alluredeck/compare/v0.33.1...v0.33.2) (2026-05-09)

### Performance Improvements

* **docker:** cross-compile Go binary natively for multi-arch builds ([18548ae](https://github.com/mkutlak/alluredeck/commit/18548ae33d5481baf5c43b7ef035f688be979c5f))

## [0.33.1](https://github.com/mkutlak/alluredeck/compare/v0.33.0...v0.33.1) (2026-05-09)

### Performance Improvements

* **upload:** parallelize tar.gz storage writes ([c53c227](https://github.com/mkutlak/alluredeck/commit/c53c227f77506fa8cea63d60903264a2e17bdbe9))

## [0.33.0](https://github.com/mkutlak/alluredeck/compare/v0.32.2...v0.33.0) (2026-05-09)

### Features

* **ui:** user-configurable timezone and 12h/24h time format ([c6a66b1](https://github.com/mkutlak/alluredeck/commit/c6a66b16dfb7005b3b7e6373f2df3abe05148b2e))

## [0.32.2](https://github.com/mkutlak/alluredeck/compare/v0.32.1...v0.32.2) (2026-05-09)

### Bug Fixes

* **branches:** derive branch list from builds across child projects ([3fae6c0](https://github.com/mkutlak/alluredeck/commit/3fae6c03db0820a0224eb1a581242e1614211843))

## [0.32.1](https://github.com/mkutlak/alluredeck/compare/v0.32.0...v0.32.1) (2026-04-29)

### Bug Fixes

* **authz:** fix is_active recheck for legacy api_key.username shapes ([927ef9e](https://github.com/mkutlak/alluredeck/commit/927ef9ef42d0ebcd90b2ba98390a9ccae3cfe791))

## [0.32.0](https://github.com/mkutlak/alluredeck/compare/v0.31.0...v0.32.0) (2026-04-27)

### Features

* **attachments:** add server-side test status filter ([64efb4b](https://github.com/mkutlak/alluredeck/commit/64efb4b8c66f582420f16c8e77bcaf5090ec95ab))

## [0.31.0](https://github.com/mkutlak/alluredeck/compare/v0.30.0...v0.31.0) (2026-04-26)

### Features

* **observability:** add OpenTelemetry traces, metrics, and Helm scrape ([1967d1a](https://github.com/mkutlak/alluredeck/commit/1967d1a1f9e7e7ceafadcb386737e53c9f82440c))

## [0.30.0](https://github.com/mkutlak/alluredeck/compare/v0.29.0...v0.30.0) (2026-04-25)

### Features

* **users:** admin user management + self-service profile ([4bcf1e4](https://github.com/mkutlak/alluredeck/commit/4bcf1e48f309e5ef826ea21cf39d1598f3dec177))

## [0.29.0](https://github.com/mkutlak/alluredeck/compare/v0.28.0...v0.29.0) (2026-04-24)

### Features

* **ui:** add project picker to webhooks page and gate sidebar link to editor+ ([71f37fe](https://github.com/mkutlak/alluredeck/commit/71f37fec78afb90f754444209b7f633fafc07fdb))

## [0.28.0](https://github.com/mkutlak/alluredeck/compare/v0.27.1...v0.28.0) (2026-04-24)

### Features

* **api:** replace Swagger UI with Scalar API reference ([4ea7c63](https://github.com/mkutlak/alluredeck/commit/4ea7c63c9d8d328be3e853c0659a735adae1f2b6))

## [0.27.1](https://github.com/mkutlak/alluredeck/compare/v0.27.0...v0.27.1) (2026-04-24)

### Bug Fixes

* **api:** query DB for stability data instead of reading files from disk ([be72e81](https://github.com/mkutlak/alluredeck/commit/be72e8108fa960bd1810770abc238add5a8b0041))

## [0.27.0](https://github.com/mkutlak/alluredeck/compare/v0.26.2...v0.27.0) (2026-04-23)

### Features

* **admin:** paginate jobs table in System Monitor ([1d8ebfc](https://github.com/mkutlak/alluredeck/commit/1d8ebfc6e73e56c1dad9c62a07aa9c1dd2dd8b9d))

## [0.26.2](https://github.com/mkutlak/alluredeck/compare/v0.26.1...v0.26.2) (2026-04-23)

### Bug Fixes

* **api:** Update dependencies ([593311c](https://github.com/mkutlak/alluredeck/commit/593311c7176218ace8e97ac01d0ee8a64979b8a9))

## [0.26.1](https://github.com/mkutlak/alluredeck/compare/v0.26.0...v0.26.1) (2026-04-22)

### Bug Fixes

* **ui:** fix attachment filters and improve styling ([2897f1e](https://github.com/mkutlak/alluredeck/commit/2897f1e3af013cf7140efc1f1cc59c32cec91e88))

## [0.26.0](https://github.com/mkutlak/alluredeck/compare/v0.25.0...v0.26.0) (2026-04-22)

### Features

* group parent project pipeline runs by CI pipeline ID ([a7a8ea6](https://github.com/mkutlak/alluredeck/commit/a7a8ea667a4bae2ef40de764a9413961c742cdf7))

## [0.25.0](https://github.com/mkutlak/alluredeck/compare/v0.24.1...v0.25.0) (2026-04-22)

### Features

* **api:** make archive file count limit configurable via MAX_ARCHIVE_FILE_COUNT ([bfd1331](https://github.com/mkutlak/alluredeck/commit/bfd1331c821f6475142a85a3473ec22cdad8ec7d))

## [0.24.1](https://github.com/mkutlak/alluredeck/compare/v0.24.0...v0.24.1) (2026-04-22)

### Bug Fixes

* isolate test results per upload batch to prevent accumulation ([4443713](https://github.com/mkutlak/alluredeck/commit/4443713ac21bbb2efd9c7ffa49743671892d358a))

## [0.24.0](https://github.com/mkutlak/alluredeck/compare/v0.23.6...v0.24.0) (2026-04-21)

### Features

* add /projects/index endpoint and group System Monitor by parent ([1775880](https://github.com/mkutlak/alluredeck/commit/177588021845054cd6f1e0df1b9deaa719525b2a))

## [0.23.6](https://github.com/mkutlak/alluredeck/compare/v0.23.5...v0.23.6) (2026-04-21)

### Bug Fixes

* prefer display_name in project labels and expose parent_id in dashboard API ([eb5aeb6](https://github.com/mkutlak/alluredeck/commit/eb5aeb630e9cd1955d2fc5f55bd9b51d651716f9))

## [0.23.5](https://github.com/mkutlak/alluredeck/compare/v0.23.4...v0.23.5) (2026-04-20)

### Bug Fixes

* resolve projects by slug/ID beyond first page, show parent/child labels ([22c61cb](https://github.com/mkutlak/alluredeck/commit/22c61cb9c102f068c8d94f764791374a10dffd6a))

## [0.23.4](https://github.com/mkutlak/alluredeck/compare/v0.23.3...v0.23.4) (2026-04-19)

### Bug Fixes

* **ui:** show slugs not project_ids; redirect on missing project ([51a4576](https://github.com/mkutlak/alluredeck/commit/51a4576ef90f4a0bd0b698320330e75b8ec08373))

## [0.23.3](https://github.com/mkutlak/alluredeck/compare/v0.23.2...v0.23.3) (2026-04-17)

### Bug Fixes

* **ui:** use numeric project_id for all project navigation links ([9e053b3](https://github.com/mkutlak/alluredeck/commit/9e053b33994e903ddfcba1958dbbefdfa7ab3d21))

## [0.23.2](https://github.com/mkutlak/alluredeck/compare/v0.23.1...v0.23.2) (2026-04-17)

### Bug Fixes

* resolve storage_key for Allure report serving and add missing slug to pipeline response ([404f3ee](https://github.com/mkutlak/alluredeck/commit/404f3ee00d8b08add16e22e710993394c3554661))

## [0.23.1](https://github.com/mkutlak/alluredeck/compare/v0.23.0...v0.23.1) (2026-04-17)

### Bug Fixes

* **api:** resolve storage_key for Playwright reports and pending results ([cf6e4f3](https://github.com/mkutlak/alluredeck/commit/cf6e4f352d74c4fe366f8fbab990eff5226cf1f6))

## [0.23.0](https://github.com/mkutlak/alluredeck/compare/v0.22.6...v0.23.0) (2026-04-16)

### Features

* **api:** introduce storage_key to decouple child project storage from slug ([89037fe](https://github.com/mkutlak/alluredeck/commit/89037fe7d57a957614290cee5fb872a138f82d1a))

## [0.22.6](https://github.com/mkutlak/alluredeck/compare/v0.22.5...v0.22.6) (2026-04-15)

### Bug Fixes

* **api:** raise HTTP server and Playwright upload timeouts ([c329f2f](https://github.com/mkutlak/alluredeck/commit/c329f2f3526ce428f4593a581273bb4d81b0c983))
* **ui:** align Report #N breadcrumb with sibling ghost button ([82281a0](https://github.com/mkutlak/alluredeck/commit/82281a0de7c4c9ac90fb5baa707942e976e09371)), closes [#N](https://github.com/mkutlak/alluredeck/issues/N)

## [0.22.5](https://github.com/mkutlak/alluredeck/compare/v0.22.4...v0.22.5) (2026-04-14)

### Bug Fixes

* add REPORT_GENERATION_TIMEOUT and rename chart containers ([f3f4694](https://github.com/mkutlak/alluredeck/commit/f3f46949d2f9e60ec5b7717f4890d45a7e955af9))

## [0.22.4](https://github.com/mkutlak/alluredeck/compare/v0.22.3...v0.22.4) (2026-04-14)

### Bug Fixes

* branch grouping, CSRF refresh, Allure 3 duration, and report viewer home link ([f7f688f](https://github.com/mkutlak/alluredeck/commit/f7f688f3fae9af7992cf2e7c9b55127dcc3d0a7a))

## [0.22.3](https://github.com/mkutlak/alluredeck/compare/v0.22.2...v0.22.3) (2026-04-13)

### Bug Fixes

* **api:** dedupe top-level project rows after surrogate-PK migration ([8ef86aa](https://github.com/mkutlak/alluredeck/commit/8ef86aa3d3ad1f59034764495b2fbbb70f0357f5))

## [0.22.2](https://github.com/mkutlak/alluredeck/compare/v0.22.1...v0.22.2) (2026-04-13)

### Bug Fixes

* **ui:** default branch filter to "All branches" and persist across projects ([d66811e](https://github.com/mkutlak/alluredeck/commit/d66811ed7d7a588ea9ac0eae315fda50991c56f1))

## [0.22.1](https://github.com/mkutlak/alluredeck/compare/v0.22.0...v0.22.1) (2026-04-12)

### Bug Fixes

* **api:** resolve nested child projects by slug in handlers ([f9fd7f6](https://github.com/mkutlak/alluredeck/commit/f9fd7f6c39d1a466dab32455047617ff84792540))

## [0.22.0](https://github.com/mkutlak/alluredeck/compare/v0.21.0...v0.22.0) (2026-04-12)

### Features

* proactive token refresh, E2E stability improvements, and security hardening ([cfe6962](https://github.com/mkutlak/alluredeck/commit/cfe6962b1ec1389aaca63758dba8486c4126be79))

## [0.21.0](https://github.com/mkutlak/alluredeck/compare/v0.20.0...v0.21.0) (2026-04-12)

### Features

* **auth:** sliding sessions with rotating refresh tokens + session drift fix ([60e5806](https://github.com/mkutlak/alluredeck/commit/60e5806e2e69381e9e4371a872726363fac917d0))
* surrogate PKs, DnD project grouping, pending results improvements, and bug fixes ([7561f21](https://github.com/mkutlak/alluredeck/commit/7561f21c4c2e64e21b6649053469817cbaf5aa0a))

## [0.20.0](https://github.com/mkutlak/alluredeck/compare/v0.19.0...v0.20.0) (2026-04-07)

### Features

* add user preferences API, dashboard redesign, and Playwright report improvements ([e2944d4](https://github.com/mkutlak/alluredeck/commit/e2944d4d004fc38a50f6c40da0bbfd23fd843b06))

### Bug Fixes

* update AppSidebar test to reflect Administration section visible to all users ([fdfafeb](https://github.com/mkutlak/alluredeck/commit/fdfafeb4942524d143de9fd60f1461f2cc62fc5e))

## [0.19.0](https://github.com/mkutlak/alluredeck/compare/v0.18.1...v0.19.0) (2026-04-05)

### Features

* add Playwright CI metadata support, E2E test suite, and rename BuildOrder to BuildNumber ([8d3bb7c](https://github.com/mkutlak/alluredeck/commit/8d3bb7ca95fc863fcbe2107c471e66b1e761bc4f))

## [0.18.1](https://github.com/mkutlak/alluredeck/compare/v0.18.0...v0.18.1) (2026-04-04)

### Bug Fixes

* report_id returns build order, UI improvements ([65d52e2](https://github.com/mkutlak/alluredeck/commit/65d52e2854a770f3dfbe846d6fdf593e60f124b6))

## [0.18.0](https://github.com/mkutlak/alluredeck/compare/v0.17.1...v0.18.0) (2026-04-04)

### Features

* add Pipeline Runs view for parent projects and simplify dashboard ([d768bb9](https://github.com/mkutlak/alluredeck/commit/d768bb928837b817c8b3a415078892b869b6da3f))
* hybrid Playwright + Allure report pipeline ([04db73c](https://github.com/mkutlak/alluredeck/commit/04db73c3971143cbfba9694ee2c0d6aa8c009a2a))

## [0.17.1](https://github.com/mkutlak/alluredeck/compare/v0.17.0...v0.17.1) (2026-04-03)

### Bug Fixes

* **api:** resolve Playwright S3 upload timeout and add parent project features ([26814dc](https://github.com/mkutlak/alluredeck/commit/26814dcefd3c949f383a54f216b07ba42ec3cc20))

## [0.17.0](https://github.com/mkutlak/alluredeck/compare/v0.16.0...v0.17.0) (2026-04-03)

### Features

* **api:** auto-generate reports on result upload and fix Playwright S3 uploads ([830e48e](https://github.com/mkutlak/alluredeck/commit/830e48e57182d5d2a0524e0e46b87269eded3af6))

## [0.16.0](https://github.com/mkutlak/alluredeck/compare/v0.15.0...v0.16.0) (2026-04-02)

### Features

* add report_type to projects for Allure/Playwright distinction ([1dc0c16](https://github.com/mkutlak/alluredeck/commit/1dc0c162beddcf3418d1c80e12cacd39513e8c91))

## [0.15.0](https://github.com/mkutlak/alluredeck/compare/v0.14.2...v0.15.0) (2026-04-02)

### Features

* **api:** add Playwright HTML report upload support ([1764e4a](https://github.com/mkutlak/alluredeck/commit/1764e4accf279510730db3a733fc27596222292e))

## [0.14.2](https://github.com/mkutlak/alluredeck/compare/v0.14.1...v0.14.2) (2026-04-01)

### Bug Fixes

* **helm,ui:** simplify ingress paths and fix lightbox overflow ([fda38eb](https://github.com/mkutlak/alluredeck/commit/fda38ebe92a248769a28418f36e2534c58a2c75d))

## [0.14.1](https://github.com/mkutlak/alluredeck/compare/v0.14.0...v0.14.1) (2026-04-01)

### Bug Fixes

* detect Playwright trace attachments without .zip name suffix ([c0026ed](https://github.com/mkutlak/alluredeck/commit/c0026ed0f9cc5972ecb13b29868c93e4f8a42a4e))

## [0.14.0](https://github.com/mkutlak/alluredeck/compare/v0.13.0...v0.14.0) (2026-03-31)

### Features

* compact attachment rows with MIME badges and fix defect fingerprinting ([82bff45](https://github.com/mkutlak/alluredeck/commit/82bff45487f41b4f08b20c6227835795cd81274c))

## [0.13.0](https://github.com/mkutlak/alluredeck/compare/v0.12.0...v0.13.0) (2026-03-31)

### Features

* add embedded Playwright trace viewer for test attachments ([f012c90](https://github.com/mkutlak/alluredeck/commit/f012c901e815458e41577941aad75abe80fe1a25))

### Bug Fixes

* harden API security and standardize response envelope ([269fdc0](https://github.com/mkutlak/alluredeck/commit/269fdc026951043b76498d1e3e8db42012adb3e7))

## [0.12.0](https://github.com/mkutlak/alluredeck/compare/v0.11.0...v0.12.0) (2026-03-29)

### Features

* add webhook notifications for Slack, Discord, Teams, and generic HTTP ([d9496ab](https://github.com/mkutlak/alluredeck/commit/d9496ab884d9accad7cdf1e6445ece0e84e30349))

### Bug Fixes

* resolve attachment display issues and add text preview ([68cabd8](https://github.com/mkutlak/alluredeck/commit/68cabd883fffba73030dfb7615d6582465290865))

## [0.11.0](https://github.com/mkutlak/alluredeck/compare/v0.10.1...v0.11.0) (2026-03-29)

### Features

* **db,store:** add defect fingerprinting schema and DefectStorer interface ([9b08b2d](https://github.com/mkutlak/alluredeck/commit/9b08b2d863efd0a679f1ac72ab04e15a0edefa81))
* **handlers:** add DefectHandler with all endpoints and route registration ([b0f60e1](https://github.com/mkutlak/alluredeck/commit/b0f60e1d207bd9ddd103444b49cdd0ad7a4fa37f))
* **runner:** add fingerprint normalization, hashing, and heuristic categorization ([5d03581](https://github.com/mkutlak/alluredeck/commit/5d0358187b64eb28407487a81520b5616242a92d))
* **store,runner:** add PGDefectStore, mock, and runner integration ([106aee0](https://github.com/mkutlak/alluredeck/commit/106aee01885f8165ac2e5768aa4586aa0aba7802))
* **ui:** add complete defect grouping frontend ([90bfc27](https://github.com/mkutlak/alluredeck/commit/90bfc2707cd314f5f750cfac19a8888e5ac29acb))

### Bug Fixes

* resolve lint issues in defect store and fingerprint tests ([b871823](https://github.com/mkutlak/alluredeck/commit/b8718231a1e8abb7d56fd77a6a4bf837d63f00ed))

## [0.10.1](https://github.com/mkutlak/alluredeck/compare/v0.10.0...v0.10.1) (2026-03-28)

### Bug Fixes

* resolve cache invalidation and auth persistence issues ([814b279](https://github.com/mkutlak/alluredeck/commit/814b279e64c9bde9aa1852579be8d60aa2b35279))

## [0.10.0](https://github.com/mkutlak/alluredeck/compare/v0.9.1...v0.10.0) (2026-03-27)

### Features

* implement project hierarchy and renaming ([aa60e4c](https://github.com/mkutlak/alluredeck/commit/aa60e4ca62afe25364d86caef63efa44f28822b6))

## [0.9.1](https://github.com/mkutlak/alluredeck/compare/v0.9.0...v0.9.1) (2026-03-26)

### Bug Fixes

* **ui:** update sidebar collapsible icon data attributes ([3398cd9](https://github.com/mkutlak/alluredeck/commit/3398cd9b87e68a471f5d44e8bd3c976f5f2a209c))

## [0.9.0](https://github.com/mkutlak/alluredeck/compare/v0.8.0...v0.9.0) (2026-03-26)

### Features

* implement multi-build timeline view ([2634f99](https://github.com/mkutlak/alluredeck/commit/2634f998b552a2eb17bf12a1893fe14360c3e961))

## [0.8.0](https://github.com/mkutlak/alluredeck/compare/v0.7.0...v0.8.0) (2026-03-15)

### Features

* **analytics:** implement advanced analytics trends and build retention ([d1ee996](https://github.com/mkutlak/alluredeck/commit/d1ee99604c52cb0550de6d191e942d3dd087fd90))
* **attachments:** implement report attachment viewer and browser ([a06e47c](https://github.com/mkutlak/alluredeck/commit/a06e47cfa0807b1b4d7d9ab44d041a2bbc11254c))
* **auth:** implement API key management and authentication ([cc3888e](https://github.com/mkutlak/alluredeck/commit/cc3888e12baede94b6f717df0ae757ba4e1b15ee))
* **auth:** implement OIDC SSO support and multi-role user management ([472084b](https://github.com/mkutlak/alluredeck/commit/472084bc978276d0860b62c0025ff59f9d012eac))
* **auth:** increase default JWT access token expiration to 1 hour ([5572460](https://github.com/mkutlak/alluredeck/commit/557246006a83f15d690aa8bd1b5fe7c3247bc02d))
* **ui:** implement report pagination settings and grouping ([76aaab6](https://github.com/mkutlak/alluredeck/commit/76aaab6e8648b973bc8611579f68e163690b3aed))

## [0.7.0](https://github.com/mkutlak/alluredeck/compare/v0.6.0...v0.7.0) (2026-03-12)

### Features

* **ui:** refactor architecture and enhance report history management ([93c5db3](https://github.com/mkutlak/alluredeck/commit/93c5db3f685b0344ee484b54fba300bdf285f75d))

## [0.6.0](https://github.com/mkutlak/alluredeck/compare/v0.5.0...v0.6.0) (2026-03-11)

### Features

* improve report history organization and update documentation ([a5ca81a](https://github.com/mkutlak/alluredeck/commit/a5ca81a5efe420edf8eefe0f06930b5ec04ffe13))

## [0.5.0](https://github.com/mkutlak/alluredeck/compare/v0.4.0...v0.5.0) (2026-03-10)

### Features

* add admin system monitor for job and results management ([7153bcf](https://github.com/mkutlak/alluredeck/commit/7153bcf0e0294cd07742c428115233061abf2eb3))
* implement branch management and per-test history tracking ([5ac3e2f](https://github.com/mkutlak/alluredeck/commit/5ac3e2ffbdf6a594aa89b71278ff18effc458057))
* implement build comparison (diff view) across API and UI ([0e08ead](https://github.com/mkutlak/alluredeck/commit/0e08eadba9cbbb94f1b2df1b53c148ec27abdc4e))
* migrate to PostgreSQL analytics and upgrade UI theme ([9722299](https://github.com/mkutlak/alluredeck/commit/9722299ef5b210a601a944058636e356d6871787))

## [0.4.0](https://github.com/mkutlak/alluredeck/compare/v0.3.0...v0.4.0) (2026-03-07)

### Features

* add project tagging and filtering ([7f965e1](https://github.com/mkutlak/alluredeck/commit/7f965e12a2a7be7e205b0f58168df5d217579b1a))
* derive Allure 3 report timing and improve UI empty states ([2dcdee9](https://github.com/mkutlak/alluredeck/commit/2dcdee9208fa2e16094767cc7d91e07d953551ef))

### Performance Improvements

* **api:** optimize S3 storage operations and clean up linter findings ([6c03e26](https://github.com/mkutlak/alluredeck/commit/6c03e2629ed13f74474ba7d442e3f4538068db89))
* **ui:** optimize rendering and implement lazy loading ([2bdd192](https://github.com/mkutlak/alluredeck/commit/2bdd192bdfa7a83351b17dff72a540c31127a61a))

## [0.3.0](https://github.com/mkutlak/alluredeck/compare/v0.2.1...v0.3.0) (2026-03-06)

### Features

* enhance API configuration and deployment flexibility ([ab22f3e](https://github.com/mkutlak/alluredeck/commit/ab22f3e7a3ccf74c6f05248df194cb6c8c0ecdeb))

## [0.2.1](https://github.com/mkutlak/alluredeck/compare/v0.2.0...v0.2.1) (2026-03-06)

### Bug Fixes

* **api:** Fix typo in strings import ([754a97c](https://github.com/mkutlak/alluredeck/commit/754a97c2759de65f88772ce3ef14b6d0dfb1482b))
* **api:** relax CSP for Swagger UI routes ([53e67ac](https://github.com/mkutlak/alluredeck/commit/53e67acf50d7e1130ac9b379b0b4f0459bc26452))

## [0.2.0](https://github.com/mkutlak/alluredeck/compare/v0.1.2...v0.2.0) (2026-03-05)

### Features

* **chart:** add ArtifactHub integration for Helm chart discovery ([36836d8](https://github.com/mkutlak/alluredeck/commit/36836d84129ee9a1e4779b309340736f63e59715))

### Bug Fixes

* **chart:** unify ingress and add advanced pod customization ([103688c](https://github.com/mkutlak/alluredeck/commit/103688c93f9f99762fd206ebc5ab2676e539dfcb))

## [0.1.2](https://github.com/mkutlak/alluredeck/compare/v0.1.1...v0.1.2) (2026-03-04)

### Bug Fixes

* **ci:** correct semantic-release action output names in release workflow ([f3fe970](https://github.com/mkutlak/alluredeck/commit/f3fe970ef1e5909fc5d060c5289b553c3a127d6c))

## [0.1.1](https://github.com/mkutlak/alluredeck/compare/v0.1.0...v0.1.1) (2026-03-04)

### Bug Fixes

* update branch protection docs and relax ruleset for semantic-release ([110f829](https://github.com/mkutlak/alluredeck/commit/110f829807e60b61dcdce69e79cb7b59a69799ed))

## [0.1.0](https://github.com/mkutlak/alluredeck/compare/v0.0.0...v0.1.0) (2026-03-04)

### ⚠ BREAKING CHANGES

* API route prefix changed from /allure-docker-service to /api/v1

### Features

* add Helm chart for AllureDeck deployment ([1ee1e5d](https://github.com/mkutlak/alluredeck/commit/1ee1e5d112a48953585b15bb936f8a68b0ca5462))
* **api/ui:** capture and display CI metadata for reports ([7d517e1](https://github.com/mkutlak/alluredeck/commit/7d517e10134a25c7b676557b26febfae10179313))
* **api/ui:** implement advanced report features and known issues management ([7af49ff](https://github.com/mkutlak/alluredeck/commit/7af49ffa26ab0f134f3b63fad45603e537d11783))
* **api/ui:** implement security hardening and RBAC ([c21911c](https://github.com/mkutlak/alluredeck/commit/c21911c686659532e47d5e2466a96db32785d9e1))
* **api/ui:** implement test stability tracking and performance analytics ([0699ef3](https://github.com/mkutlak/alluredeck/commit/0699ef3b905fd8fe2dee868de0c5d994d15b21d9))
* **api/ui:** refactor REST API and implement pagination ([8d58f05](https://github.com/mkutlak/alluredeck/commit/8d58f05163ba77d58e0ac23a0f3b2d7328526b6a))
* **api:** implement structured logging using zap ([186ea68](https://github.com/mkutlak/alluredeck/commit/186ea6827b332abe0570d5e879eb5855414bd2a5))
* **api:** implement tar.gz archive support for test results upload ([274a4c7](https://github.com/mkutlak/alluredeck/commit/274a4c7c907df3735a5be6646a76c4bb2ed5898b))
* display application version in the UI sidebar footer ([95488f3](https://github.com/mkutlak/alluredeck/commit/95488f3a8001e242aa85bc2065833ac6244fe820))
* enhance Projects Dashboard with administrative controls and improved navigation ([1bd37a4](https://github.com/mkutlak/alluredeck/commit/1bd37a4bc2a60cdde984c933c97790adb7189896))
* **helm:** refactor chart structure for per-service ingress and improved secret handling ([e03fc73](https://github.com/mkutlak/alluredeck/commit/e03fc73315d595ca631e415e202b73d69ac43936))
* **helm:** switch API to file-based config and improve chart robustness ([316e86c](https://github.com/mkutlak/alluredeck/commit/316e86c51abac3b15053520838ab63d2830a04a7))
* implement async report generation with job manager and polling ([24cb47d](https://github.com/mkutlak/alluredeck/commit/24cb47def2c2437efe3d74d24ab5ea5c182e224a))
* implement cross-project dashboard with pass rate sparklines ([9d54917](https://github.com/mkutlak/alluredeck/commit/9d54917a6b19b25710a1ab76db5de0579843945b))
* implement global search for projects and test results ([c192dba](https://github.com/mkutlak/alluredeck/commit/c192dba6b3d75912b1a4f9c0b757a4d3662e1317))
* implement modern sidebar navigation and enhance search UI ([cbc0fc3](https://github.com/mkutlak/alluredeck/commit/cbc0fc36c1f621cb01f6ac0c86b59e38825ecc43))
* implement pagination for report history and inline status toggle for known issues ([f9af46c](https://github.com/mkutlak/alluredeck/commit/f9af46c29c2fda4c26fb63b07d7c507fa78d184b))
* implement report summary and remove emailable report ([faf2c6b](https://github.com/mkutlak/alluredeck/commit/faf2c6b35bf5b5783ac0f5df7e060f1f55e2e6fd))
* modernize CI/CD workflows and enhance test environment ([04b7997](https://github.com/mkutlak/alluredeck/commit/04b7997734ed6ca864205c4b78ed9a1d1fb71e9d))
* modernize core stack to React 19, React Router 7, and Tailwind CSS 4 ([6c91449](https://github.com/mkutlak/alluredeck/commit/6c914491ecb86821ea41c95268b8826994a63cdf))
* refactor charts to shadcn-style and enhance sidebar navigation ([ee98911](https://github.com/mkutlak/alluredeck/commit/ee98911107abeb4fad1314d93200b8922b3f6968))
* reorganize into AllureDeck monorepo ([94b9142](https://github.com/mkutlak/alluredeck/commit/94b9142ada3ae774c7636273a9722d7c6739af16))
* **reports:** improve report history management and upload workflow ([e165ccf](https://github.com/mkutlak/alluredeck/commit/e165ccf39c5aede45758ce81c356ecb0f2e7aea9))
* **security:** implement bcrypt hashing, project isolation, and XSS protection ([615f3bd](https://github.com/mkutlak/alluredeck/commit/615f3bdc93d2b948684bfbcdca712053c46c60f2))
* **ui:** add failure category breakdown chart to analytics tab ([4a3c5f8](https://github.com/mkutlak/alluredeck/commit/4a3c5f8b210d95dcbb91d9ca3742ca1ddb538579))
* **ui:** dashboard redesign ([b87ca2f](https://github.com/mkutlak/alluredeck/commit/b87ca2fe501527eaca4b2904b9b252a8d3514e4d))

### Bug Fixes

* configure semantic-release with conventionalcommits and fix tag creation ([5cd434d](https://github.com/mkutlak/alluredeck/commit/5cd434d62e1c3b8ea62cf84315ac61987e1bbc3a))
* **ui/docker:** update API URL and fix project fetching ([8f026fd](https://github.com/mkutlak/alluredeck/commit/8f026fdf5637cbd2a0ec04bca0c5ea3e787c9582))

### Performance Improvements

* **api:** add covering index for test results analytics ([5a6b46a](https://github.com/mkutlak/alluredeck/commit/5a6b46a4081a0135bf4c3eb6c9b47d6f412d6972))
* **api:** implement HTTP cache-control middleware and apply to routes ([0d0c78f](https://github.com/mkutlak/alluredeck/commit/0d0c78fdaae2e5d71cfe2e00d6b8124fbbfbc403))
* **api:** implement parallel S3 uploads and downloads with bounded concurrency ([91307d8](https://github.com/mkutlak/alluredeck/commit/91307d8092f4ff62064ae5f69ae5699264c47806))
* **api:** implement SQLite fast path for report timeline ([9c11e68](https://github.com/mkutlak/alluredeck/commit/9c11e68c12e689994dbd933be090b5035a92b8d9))
* **api:** optimize metadata synchronization with batching and concurrency ([a6f9f0f](https://github.com/mkutlak/alluredeck/commit/a6f9f0f6f85fae63e3fa7170e7d6006b8a7a8e0c))
* **api:** optimize S3 history persistence using CopyObject ([cb3687a](https://github.com/mkutlak/alluredeck/commit/cb3687a72951e9814b9a0816e2ebd6b3a96de211))
* **api:** optimize SetLatest by clearing previous latest flag with targeted query ([7a9b69c](https://github.com/mkutlak/alluredeck/commit/7a9b69c426d0d4885a0d38c61d29d4002b22cbf7))
* **api:** optimize test trend fetching by batching queries ([a675fff](https://github.com/mkutlak/alluredeck/commit/a675fffdf4d72543ef1ad09811e93f34fbfc8deb))
* **docker:** set GOMEMLIMIT to 1GiB for API containers ([843cff2](https://github.com/mkutlak/alluredeck/commit/843cff210f6aab0278223a58c4cac55aaa7e7086))

# [1.0.0](https://github.com/mkutlak/alluredeck/compare/v0.0.0...v1.0.0) (2026-03-04)


### Bug Fixes

* **ui/docker:** update API URL and fix project fetching ([8f026fd](https://github.com/mkutlak/alluredeck/commit/8f026fdf5637cbd2a0ec04bca0c5ea3e787c9582))


### Features

* add Helm chart for AllureDeck deployment ([1ee1e5d](https://github.com/mkutlak/alluredeck/commit/1ee1e5d112a48953585b15bb936f8a68b0ca5462))
* **api/ui:** capture and display CI metadata for reports ([7d517e1](https://github.com/mkutlak/alluredeck/commit/7d517e10134a25c7b676557b26febfae10179313))
* **api/ui:** implement advanced report features and known issues management ([7af49ff](https://github.com/mkutlak/alluredeck/commit/7af49ffa26ab0f134f3b63fad45603e537d11783))
* **api/ui:** implement security hardening and RBAC ([c21911c](https://github.com/mkutlak/alluredeck/commit/c21911c686659532e47d5e2466a96db32785d9e1))
* **api/ui:** implement test stability tracking and performance analytics ([0699ef3](https://github.com/mkutlak/alluredeck/commit/0699ef3b905fd8fe2dee868de0c5d994d15b21d9))
* **api/ui:** refactor REST API and implement pagination ([8d58f05](https://github.com/mkutlak/alluredeck/commit/8d58f05163ba77d58e0ac23a0f3b2d7328526b6a))
* **api:** implement structured logging using zap ([186ea68](https://github.com/mkutlak/alluredeck/commit/186ea6827b332abe0570d5e879eb5855414bd2a5))
* **api:** implement tar.gz archive support for test results upload ([274a4c7](https://github.com/mkutlak/alluredeck/commit/274a4c7c907df3735a5be6646a76c4bb2ed5898b))
* display application version in the UI sidebar footer ([95488f3](https://github.com/mkutlak/alluredeck/commit/95488f3a8001e242aa85bc2065833ac6244fe820))
* enhance Projects Dashboard with administrative controls and improved navigation ([1bd37a4](https://github.com/mkutlak/alluredeck/commit/1bd37a4bc2a60cdde984c933c97790adb7189896))
* **helm:** refactor chart structure for per-service ingress and improved secret handling ([e03fc73](https://github.com/mkutlak/alluredeck/commit/e03fc73315d595ca631e415e202b73d69ac43936))
* **helm:** switch API to file-based config and improve chart robustness ([316e86c](https://github.com/mkutlak/alluredeck/commit/316e86c51abac3b15053520838ab63d2830a04a7))
* implement async report generation with job manager and polling ([24cb47d](https://github.com/mkutlak/alluredeck/commit/24cb47def2c2437efe3d74d24ab5ea5c182e224a))
* implement cross-project dashboard with pass rate sparklines ([9d54917](https://github.com/mkutlak/alluredeck/commit/9d54917a6b19b25710a1ab76db5de0579843945b))
* implement global search for projects and test results ([c192dba](https://github.com/mkutlak/alluredeck/commit/c192dba6b3d75912b1a4f9c0b757a4d3662e1317))
* implement modern sidebar navigation and enhance search UI ([cbc0fc3](https://github.com/mkutlak/alluredeck/commit/cbc0fc36c1f621cb01f6ac0c86b59e38825ecc43))
* implement pagination for report history and inline status toggle for known issues ([f9af46c](https://github.com/mkutlak/alluredeck/commit/f9af46c29c2fda4c26fb63b07d7c507fa78d184b))
* implement report summary and remove emailable report ([faf2c6b](https://github.com/mkutlak/alluredeck/commit/faf2c6b35bf5b5783ac0f5df7e060f1f55e2e6fd))
* modernize CI/CD workflows and enhance test environment ([04b7997](https://github.com/mkutlak/alluredeck/commit/04b7997734ed6ca864205c4b78ed9a1d1fb71e9d))
* modernize core stack to React 19, React Router 7, and Tailwind CSS 4 ([6c91449](https://github.com/mkutlak/alluredeck/commit/6c914491ecb86821ea41c95268b8826994a63cdf))
* refactor charts to shadcn-style and enhance sidebar navigation ([ee98911](https://github.com/mkutlak/alluredeck/commit/ee98911107abeb4fad1314d93200b8922b3f6968))
* reorganize into AllureDeck monorepo ([94b9142](https://github.com/mkutlak/alluredeck/commit/94b9142ada3ae774c7636273a9722d7c6739af16))
* **reports:** improve report history management and upload workflow ([e165ccf](https://github.com/mkutlak/alluredeck/commit/e165ccf39c5aede45758ce81c356ecb0f2e7aea9))
* **security:** implement bcrypt hashing, project isolation, and XSS protection ([615f3bd](https://github.com/mkutlak/alluredeck/commit/615f3bdc93d2b948684bfbcdca712053c46c60f2))
* **ui:** add failure category breakdown chart to analytics tab ([4a3c5f8](https://github.com/mkutlak/alluredeck/commit/4a3c5f8b210d95dcbb91d9ca3742ca1ddb538579))
* **ui:** dashboard redesign ([b87ca2f](https://github.com/mkutlak/alluredeck/commit/b87ca2fe501527eaca4b2904b9b252a8d3514e4d))


### Performance Improvements

* **api:** add covering index for test results analytics ([5a6b46a](https://github.com/mkutlak/alluredeck/commit/5a6b46a4081a0135bf4c3eb6c9b47d6f412d6972))
* **api:** implement HTTP cache-control middleware and apply to routes ([0d0c78f](https://github.com/mkutlak/alluredeck/commit/0d0c78fdaae2e5d71cfe2e00d6b8124fbbfbc403))
* **api:** implement parallel S3 uploads and downloads with bounded concurrency ([91307d8](https://github.com/mkutlak/alluredeck/commit/91307d8092f4ff62064ae5f69ae5699264c47806))
* **api:** implement SQLite fast path for report timeline ([9c11e68](https://github.com/mkutlak/alluredeck/commit/9c11e68c12e689994dbd933be090b5035a92b8d9))
* **api:** optimize metadata synchronization with batching and concurrency ([a6f9f0f](https://github.com/mkutlak/alluredeck/commit/a6f9f0f6f85fae63e3fa7170e7d6006b8a7a8e0c))
* **api:** optimize S3 history persistence using CopyObject ([cb3687a](https://github.com/mkutlak/alluredeck/commit/cb3687a72951e9814b9a0816e2ebd6b3a96de211))
* **api:** optimize SetLatest by clearing previous latest flag with targeted query ([7a9b69c](https://github.com/mkutlak/alluredeck/commit/7a9b69c426d0d4885a0d38c61d29d4002b22cbf7))
* **api:** optimize test trend fetching by batching queries ([a675fff](https://github.com/mkutlak/alluredeck/commit/a675fffdf4d72543ef1ad09811e93f34fbfc8deb))
* **docker:** set GOMEMLIMIT to 1GiB for API containers ([843cff2](https://github.com/mkutlak/alluredeck/commit/843cff210f6aab0278223a58c4cac55aaa7e7086))


### BREAKING CHANGES

* API route prefix changed from /allure-docker-service to /api/v1

# 1.0.0 (2026-03-03)


### Bug Fixes

* **ui/docker:** update API URL and fix project fetching ([8f026fd](https://github.com/mkutlak/alluredeck/commit/8f026fdf5637cbd2a0ec04bca0c5ea3e787c9582))


### Features

* add Helm chart for AllureDeck deployment ([1ee1e5d](https://github.com/mkutlak/alluredeck/commit/1ee1e5d112a48953585b15bb936f8a68b0ca5462))
* **api/ui:** capture and display CI metadata for reports ([7d517e1](https://github.com/mkutlak/alluredeck/commit/7d517e10134a25c7b676557b26febfae10179313))
* **api/ui:** implement advanced report features and known issues management ([7af49ff](https://github.com/mkutlak/alluredeck/commit/7af49ffa26ab0f134f3b63fad45603e537d11783))
* **api/ui:** implement security hardening and RBAC ([c21911c](https://github.com/mkutlak/alluredeck/commit/c21911c686659532e47d5e2466a96db32785d9e1))
* **api/ui:** implement test stability tracking and performance analytics ([0699ef3](https://github.com/mkutlak/alluredeck/commit/0699ef3b905fd8fe2dee868de0c5d994d15b21d9))
* **api/ui:** refactor REST API and implement pagination ([8d58f05](https://github.com/mkutlak/alluredeck/commit/8d58f05163ba77d58e0ac23a0f3b2d7328526b6a))
* **api:** implement structured logging using zap ([186ea68](https://github.com/mkutlak/alluredeck/commit/186ea6827b332abe0570d5e879eb5855414bd2a5))
* **api:** implement tar.gz archive support for test results upload ([274a4c7](https://github.com/mkutlak/alluredeck/commit/274a4c7c907df3735a5be6646a76c4bb2ed5898b))
* display application version in the UI sidebar footer ([95488f3](https://github.com/mkutlak/alluredeck/commit/95488f3a8001e242aa85bc2065833ac6244fe820))
* enhance Projects Dashboard with administrative controls and improved navigation ([1bd37a4](https://github.com/mkutlak/alluredeck/commit/1bd37a4bc2a60cdde984c933c97790adb7189896))
* **helm:** refactor chart structure for per-service ingress and improved secret handling ([e03fc73](https://github.com/mkutlak/alluredeck/commit/e03fc73315d595ca631e415e202b73d69ac43936))
* **helm:** switch API to file-based config and improve chart robustness ([316e86c](https://github.com/mkutlak/alluredeck/commit/316e86c51abac3b15053520838ab63d2830a04a7))
* implement async report generation with job manager and polling ([24cb47d](https://github.com/mkutlak/alluredeck/commit/24cb47def2c2437efe3d74d24ab5ea5c182e224a))
* implement cross-project dashboard with pass rate sparklines ([9d54917](https://github.com/mkutlak/alluredeck/commit/9d54917a6b19b25710a1ab76db5de0579843945b))
* implement global search for projects and test results ([c192dba](https://github.com/mkutlak/alluredeck/commit/c192dba6b3d75912b1a4f9c0b757a4d3662e1317))
* implement modern sidebar navigation and enhance search UI ([cbc0fc3](https://github.com/mkutlak/alluredeck/commit/cbc0fc36c1f621cb01f6ac0c86b59e38825ecc43))
* implement pagination for report history and inline status toggle for known issues ([f9af46c](https://github.com/mkutlak/alluredeck/commit/f9af46c29c2fda4c26fb63b07d7c507fa78d184b))
* implement report summary and remove emailable report ([faf2c6b](https://github.com/mkutlak/alluredeck/commit/faf2c6b35bf5b5783ac0f5df7e060f1f55e2e6fd))
* modernize core stack to React 19, React Router 7, and Tailwind CSS 4 ([6c91449](https://github.com/mkutlak/alluredeck/commit/6c914491ecb86821ea41c95268b8826994a63cdf))
* refactor charts to shadcn-style and enhance sidebar navigation ([ee98911](https://github.com/mkutlak/alluredeck/commit/ee98911107abeb4fad1314d93200b8922b3f6968))
* reorganize into AllureDeck monorepo ([94b9142](https://github.com/mkutlak/alluredeck/commit/94b9142ada3ae774c7636273a9722d7c6739af16))
* **reports:** improve report history management and upload workflow ([e165ccf](https://github.com/mkutlak/alluredeck/commit/e165ccf39c5aede45758ce81c356ecb0f2e7aea9))
* **security:** implement bcrypt hashing, project isolation, and XSS protection ([615f3bd](https://github.com/mkutlak/alluredeck/commit/615f3bdc93d2b948684bfbcdca712053c46c60f2))
* **ui:** add failure category breakdown chart to analytics tab ([4a3c5f8](https://github.com/mkutlak/alluredeck/commit/4a3c5f8b210d95dcbb91d9ca3742ca1ddb538579))
* **ui:** dashboard redesign ([b87ca2f](https://github.com/mkutlak/alluredeck/commit/b87ca2fe501527eaca4b2904b9b252a8d3514e4d))


### Performance Improvements

* **api:** add covering index for test results analytics ([5a6b46a](https://github.com/mkutlak/alluredeck/commit/5a6b46a4081a0135bf4c3eb6c9b47d6f412d6972))
* **api:** implement HTTP cache-control middleware and apply to routes ([0d0c78f](https://github.com/mkutlak/alluredeck/commit/0d0c78fdaae2e5d71cfe2e00d6b8124fbbfbc403))
* **api:** implement parallel S3 uploads and downloads with bounded concurrency ([91307d8](https://github.com/mkutlak/alluredeck/commit/91307d8092f4ff62064ae5f69ae5699264c47806))
* **api:** implement SQLite fast path for report timeline ([9c11e68](https://github.com/mkutlak/alluredeck/commit/9c11e68c12e689994dbd933be090b5035a92b8d9))
* **api:** optimize metadata synchronization with batching and concurrency ([a6f9f0f](https://github.com/mkutlak/alluredeck/commit/a6f9f0f6f85fae63e3fa7170e7d6006b8a7a8e0c))
* **api:** optimize S3 history persistence using CopyObject ([cb3687a](https://github.com/mkutlak/alluredeck/commit/cb3687a72951e9814b9a0816e2ebd6b3a96de211))
* **api:** optimize SetLatest by clearing previous latest flag with targeted query ([7a9b69c](https://github.com/mkutlak/alluredeck/commit/7a9b69c426d0d4885a0d38c61d29d4002b22cbf7))
* **api:** optimize test trend fetching by batching queries ([a675fff](https://github.com/mkutlak/alluredeck/commit/a675fffdf4d72543ef1ad09811e93f34fbfc8deb))
* **docker:** set GOMEMLIMIT to 1GiB for API containers ([843cff2](https://github.com/mkutlak/alluredeck/commit/843cff210f6aab0278223a58c4cac55aaa7e7086))


### BREAKING CHANGES

* API route prefix changed from /allure-docker-service to /api/v1

# Changelog

All notable changes to AllureDeck will be documented in this file.

<!-- semantic-release appends entries above this line -->
