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
