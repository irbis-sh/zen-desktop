#!/usr/bin/env bash
#
# update-localcdn-resources.sh - download bundled local resources from
# upstream CDNs and regenerate internal/localcdn/resources.json (including
# SHA-384 SRI hashes).
#
# Requirements:
#   - curl
#   - pwsh (PowerShell 7+), used to regenerate the manifest
#
# Run from the repository root:
#   task localcdn:update-resources

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESOURCES="$ROOT/internal/localcdn/resources"

fetch() {
    local url="$1"
    local dest="$2"
    mkdir -p "$(dirname "$dest")"
    curl -fsSL --retry 3 "$url" -o "$dest"
}

# jQuery
fetch "https://code.jquery.com/jquery-1.11.3.min.js" "$RESOURCES/jquery/1.11.3/jquery.min.js"
fetch "https://code.jquery.com/jquery-2.2.4.min.js" "$RESOURCES/jquery/2.2.4/jquery.min.js"
fetch "https://code.jquery.com/jquery-3.7.1.min.js" "$RESOURCES/jquery/3.7.1/jquery.min.js"

# Bootstrap
for VERSION in 3.4.1 4.6.2 5.3.8; do
    fetch "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$VERSION/css/bootstrap.min.css" \
        "$RESOURCES/bootstrap/$VERSION/bootstrap.min.css"
    fetch "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$VERSION/js/bootstrap.min.js" \
        "$RESOURCES/bootstrap/$VERSION/bootstrap.min.js"
done

# Font Awesome 4
fetch "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.min.css" \
    "$RESOURCES/fontawesome/4.7.0/css/font-awesome.min.css"
fetch "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/fonts/fontawesome-webfont.woff2" \
    "$RESOURCES/fontawesome/4.7.0/fonts/fontawesome-webfont.woff2"

# Font Awesome 5 and 6
for VERSION in 5.15.4 6.7.2; do
    fetch "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/$VERSION/css/all.min.css" \
        "$RESOURCES/fontawesome/$VERSION/css/all.min.css"
    fetch "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/$VERSION/css/v4-shims.min.css" \
        "$RESOURCES/fontawesome/$VERSION/css/v4-shims.min.css"
    for FONT in fa-brands-400 fa-regular-400 fa-solid-900; do
        fetch "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/$VERSION/webfonts/$FONT.woff2" \
            "$RESOURCES/fontawesome/$VERSION/webfonts/$FONT.woff2"
    done
done

# Google Material Icons
# Note: Google Fonts serves this font under a hashed filename; the URL below
# is the current v145 woff2 resolved from the Material Icons stylesheet.
fetch "https://fonts.gstatic.com/s/materialicons/v145/flUhRq6tzZclQEJ-Vdg-IuiaDsNc.woff2" \
    "$RESOURCES/google-material-icons/v145/MaterialIcons-Regular.woff2"

# React and ReactDOM
fetch "https://unpkg.com/react@18.3.1/umd/react.production.min.js" \
    "$RESOURCES/react/18.3.1/react.production.min.js"
fetch "https://unpkg.com/react-dom@18.3.1/umd/react-dom.production.min.js" \
    "$RESOURCES/react/18.3.1/react-dom.production.min.js"

# Vue
fetch "https://unpkg.com/vue@2.6.14/dist/vue.min.js" "$RESOURCES/vue/2.6.14/vue.min.js"
fetch "https://unpkg.com/vue@3.5.22/dist/vue.global.prod.js" "$RESOURCES/vue/3.5.22/vue.min.js"

# Axios, Lodash, Moment.js, D3
fetch "https://unpkg.com/axios@1.11.0/dist/axios.min.js" "$RESOURCES/axios/1.11.0/axios.min.js"
fetch "https://unpkg.com/lodash@4.17.21/lodash.min.js" "$RESOURCES/lodash/4.17.21/lodash.min.js"
fetch "https://unpkg.com/moment@2.30.1/min/moment-with-locales.min.js" "$RESOURCES/moment/2.30.1/moment-with-locales.min.js"
fetch "https://unpkg.com/d3@7.9.0/dist/d3.min.js" "$RESOURCES/d3/7.9.0/d3.min.js"

# Popper.js
fetch "https://unpkg.com/popper.js@1.16.1/dist/umd/popper.min.js" "$RESOURCES/popper/1.16.1/popper.min.js"
fetch "https://unpkg.com/@popperjs/core@2.11.8/dist/umd/popper.min.js" "$RESOURCES/popper/2.11.8/popper.min.js"

# Web Font Loader and AngularJS
fetch "https://unpkg.com/webfontloader@1.6.28/webfontloader.js" "$RESOURCES/webfont/1.6.28/webfontloader.js"
fetch "https://unpkg.com/angular@1.8.2/angular.min.js" "$RESOURCES/angular/1.8.2/angular.min.js"

# Slick Carousel
fetch "https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick.min.js" \
    "$RESOURCES/slick/1.9.0/slick.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick.min.css" \
    "$RESOURCES/slick/1.9.0/slick.min.css"
fetch "https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick-theme.min.css" \
    "$RESOURCES/slick/1.9.0/slick-theme.min.css"

# Magnific Popup
fetch "https://cdnjs.cloudflare.com/ajax/libs/magnific-popup.js/1.2.0/jquery.magnific-popup.min.js" \
    "$RESOURCES/magnific-popup/1.2.0/jquery.magnific-popup.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/magnific-popup.js/1.2.0/magnific-popup.min.css" \
    "$RESOURCES/magnific-popup/1.2.0/magnific-popup.min.css"

# Highlight.js
fetch "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.11.1/highlight.min.js" \
    "$RESOURCES/highlightjs/11.11.1/highlight.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.11.1/styles/default.min.css" \
    "$RESOURCES/highlightjs/11.11.1/default.min.css"

# MathJax and Chart.js
fetch "https://cdnjs.cloudflare.com/ajax/libs/mathjax/3.2.2/es5/tex-chtml.js" \
    "$RESOURCES/mathjax/3.2.2/tex-chtml.js"
fetch "https://cdn.jsdelivr.net/npm/chart.js@4.5.0/dist/chart.umd.min.js" \
    "$RESOURCES/chartjs/4.5.0/chart.min.js"

# Swiper
fetch "https://cdn.jsdelivr.net/npm/swiper@9.4.1/swiper-bundle.min.js" \
    "$RESOURCES/swiper/9.4.1/swiper.min.js"
fetch "https://cdn.jsdelivr.net/npm/swiper@9.4.1/swiper-bundle.min.css" \
    "$RESOURCES/swiper/9.4.1/swiper.min.css"

# Web Components polyfills (Polymer)
fetch "https://unpkg.com/@webcomponents/webcomponentsjs@2.8.0/webcomponents-loader.js" \
    "$RESOURCES/polymer/2.8.0/webcomponents-loader.js"

# JavaScript Cookie
fetch "https://cdnjs.cloudflare.com/ajax/libs/js-cookie/2.2.1/js.cookie.min.js" \
    "$RESOURCES/jscookie/2.2.1/js.cookie.min.js"

# Bootstrap Slider
fetch "https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/11.0.2/bootstrap-slider.min.js" \
    "$RESOURCES/bootstrap-slider/11.0.2/bootstrap-slider.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/11.0.2/css/bootstrap-slider.min.css" \
    "$RESOURCES/bootstrap-slider/11.0.2/bootstrap-slider.min.css"

# Animate.css
fetch "https://cdnjs.cloudflare.com/ajax/libs/animate.css/4.1.1/animate.min.css" \
    "$RESOURCES/animatecss/4.1.1/animate.min.css"

# Toastr
fetch "https://cdnjs.cloudflare.com/ajax/libs/toastr.js/2.1.4/toastr.min.js" \
    "$RESOURCES/toastr/2.1.4/toastr.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/toastr.js/2.1.4/toastr.min.css" \
    "$RESOURCES/toastr/2.1.4/toastr.min.css"

# Plyr
fetch "https://cdnjs.cloudflare.com/ajax/libs/plyr/3.8.3/plyr.min.js" \
    "$RESOURCES/plyr/3.8.3/plyr.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/plyr/3.8.3/plyr.min.css" \
    "$RESOURCES/plyr/3.8.3/plyr.min.css"

# Rickshaw
fetch "https://cdnjs.cloudflare.com/ajax/libs/rickshaw/1.7.1/rickshaw.min.js" \
    "$RESOURCES/rickshaw/1.7.1/rickshaw.min.js"
fetch "https://cdnjs.cloudflare.com/ajax/libs/rickshaw/1.7.1/rickshaw.min.css" \
    "$RESOURCES/rickshaw/1.7.1/rickshaw.min.css"

# WOW.js
fetch "https://cdnjs.cloudflare.com/ajax/libs/wow/1.1.2/wow.min.js" \
    "$RESOURCES/wow/1.1.2/wow.min.js"

# jQuery jeditable
fetch "https://cdnjs.cloudflare.com/ajax/libs/jeditable.js/2.0.19/jquery.jeditable.min.js" \
    "$RESOURCES/jeditable/2.0.19/jquery.jeditable.min.js"

# jQuery Validation
fetch "https://cdnjs.cloudflare.com/ajax/libs/jquery-validate/1.21.0/jquery.validate.min.js" \
    "$RESOURCES/jqueryvalidate/1.21.0/jquery.validate.min.js"

# lazysizes
fetch "https://cdnjs.cloudflare.com/ajax/libs/lazysizes/5.3.2/lazysizes.min.js" \
    "$RESOURCES/lazysizes/5.3.2/lazysizes.min.js"

# clipboard.js
fetch "https://cdnjs.cloudflare.com/ajax/libs/clipboard.js/2.0.11/clipboard.min.js" \
    "$RESOURCES/clipboard/2.0.11/clipboard.min.js"

# P2P Media Loader Core
fetch "https://cdn.jsdelivr.net/npm/p2p-media-loader-core@0.6.2/build/p2p-media-loader-core.min.js" \
    "$RESOURCES/p2pmediacore/0.6.2/p2p-media-loader-core.min.js"

# Regenerate the manifest with SRI hashes.
if command -v pwsh >/dev/null 2>&1; then
    pwsh -NoProfile -File "$ROOT/scripts/update-localcdn-resources.ps1" -SkipDownload
else
    echo "pwsh not found; resources.json was not regenerated. Run scripts/update-localcdn-resources.ps1 manually." >&2
    exit 1
fi

echo "Bundled local resources updated."
