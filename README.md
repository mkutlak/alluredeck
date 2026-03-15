# AllureDeck — GitHub Pages

This branch (`gh-pages`) contains the documentation site for AllureDeck, served at [alluredeck.kutlak.cc](https://alluredeck.kutlak.cc).

## Structure

```
index.html          Landing page with install instructions and feature overview
features.html       Product features showcase with screenshots
docs/
  helm.html         Helm chart configuration reference
  oidc.html         OIDC authentication setup guide
css/
  style.css         Shared styles (Catppuccin Latte design language)
js/
  main.js           Shared JavaScript (copy buttons, nav, TOC tracking)
screenshots/        Product screenshots (16 images)
gopher_deck.png     Mascot image
CNAME               Custom domain (alluredeck.kutlak.cc)
.nojekyll           Bypass Jekyll processing
```

## Development

The application source code lives on the [`main`](https://github.com/mkutlak/alluredeck/tree/main) branch.

To preview locally:

```sh
python3 -m http.server 8080
# open http://localhost:8080
```
