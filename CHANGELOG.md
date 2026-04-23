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
