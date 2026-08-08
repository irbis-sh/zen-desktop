param(
    [string]$ResourceRoot = (Join-Path $PSScriptRoot '..\internal\localcdn'),
    [switch]$SkipDownload
)

<#
.SYNOPSIS
Regenerates internal/localcdn/resources.json from the bundled resource files.

.DESCRIPTION
Scans the resource tree under internal/localcdn/resources, computes SHA-384
integrity hashes, and writes the machine-readable registry manifest that the
local resource engine loads at startup.

Use this script after updating bundled library files, or run
`task localcdn:update-resources` (Linux/macOS) which delegates to the
equivalent Bash script.
#>

$ErrorActionPreference = 'Stop'
$root = $ResourceRoot
$js = 'application/javascript; charset=utf-8'
$css = 'text/css; charset=utf-8'
$woff = 'font/woff2'

# The download list below mirrors scripts/update-localcdn-resources.sh. Keep
# both in sync when adding or updating bundled libraries. On Windows the
# PowerShell script is the native updater (Git bash's MSYS2 fork emulation can
# be unreliable); pass -SkipDownload when files were already fetched.
function Invoke-Download {
    param(
        [string]$Url,
        [string]$RelativePath
    )

    $dest = Join-Path (Join-Path $root 'resources') ($RelativePath -replace '/', '\')
    New-Item -ItemType Directory -Force -Path (Split-Path $dest) | Out-Null
    curl.exe -fsSL --retry 3 $Url -o $dest
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to download $Url"
    }
}

function Update-Downloads {
    $downloads = @()
    foreach ($version in @('1.11.3', '2.2.4', '3.7.1')) {
        $downloads += ,@("https://code.jquery.com/jquery-$version.min.js", "jquery/$version/jquery.min.js")
    }
    foreach ($version in @('3.4.1', '4.6.2', '5.3.8')) {
        $downloads += ,@("https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$version/css/bootstrap.min.css", "bootstrap/$version/bootstrap.min.css")
        $downloads += ,@("https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$version/js/bootstrap.min.js", "bootstrap/$version/bootstrap.min.js")
    }
    $downloads += ,@('https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.min.css', 'fontawesome/4.7.0/css/font-awesome.min.css')
    $downloads += ,@('https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/fonts/fontawesome-webfont.woff2', 'fontawesome/4.7.0/fonts/fontawesome-webfont.woff2')
    foreach ($version in @('5.15.4', '6.7.2')) {
        $downloads += ,@("https://cdnjs.cloudflare.com/ajax/libs/font-awesome/$version/css/all.min.css", "fontawesome/$version/css/all.min.css")
        $downloads += ,@("https://cdnjs.cloudflare.com/ajax/libs/font-awesome/$version/css/v4-shims.min.css", "fontawesome/$version/css/v4-shims.min.css")
        foreach ($font in @('fa-brands-400', 'fa-regular-400', 'fa-solid-900')) {
            $downloads += ,@("https://cdnjs.cloudflare.com/ajax/libs/font-awesome/$version/webfonts/$font.woff2", "fontawesome/$version/webfonts/$font.woff2")
        }
    }
    $downloads += ,@('https://fonts.gstatic.com/s/materialicons/v145/flUhRq6tzZclQEJ-Vdg-IuiaDsNc.woff2', 'google-material-icons/v145/MaterialIcons-Regular.woff2')

    $downloads += @(
        @('https://unpkg.com/react@18.3.1/umd/react.production.min.js', 'react/18.3.1/react.production.min.js'),
        @('https://unpkg.com/react-dom@18.3.1/umd/react-dom.production.min.js', 'react/18.3.1/react-dom.production.min.js'),
        @('https://unpkg.com/vue@2.6.14/dist/vue.min.js', 'vue/2.6.14/vue.min.js'),
        @('https://unpkg.com/vue@3.5.22/dist/vue.global.prod.js', 'vue/3.5.22/vue.min.js'),
        @('https://unpkg.com/axios@1.11.0/dist/axios.min.js', 'axios/1.11.0/axios.min.js'),
        @('https://unpkg.com/lodash@4.17.21/lodash.min.js', 'lodash/4.17.21/lodash.min.js'),
        @('https://unpkg.com/moment@2.30.1/min/moment-with-locales.min.js', 'moment/2.30.1/moment-with-locales.min.js'),
        @('https://unpkg.com/d3@7.9.0/dist/d3.min.js', 'd3/7.9.0/d3.min.js'),
        @('https://unpkg.com/popper.js@1.16.1/dist/umd/popper.min.js', 'popper/1.16.1/popper.min.js'),
        @('https://unpkg.com/@popperjs/core@2.11.8/dist/umd/popper.min.js', 'popper/2.11.8/popper.min.js'),
        @('https://unpkg.com/webfontloader@1.6.28/webfontloader.js', 'webfont/1.6.28/webfontloader.js'),
        @('https://unpkg.com/angular@1.8.2/angular.min.js', 'angular/1.8.2/angular.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick.min.js', 'slick/1.9.0/slick.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick.min.css', 'slick/1.9.0/slick.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick-theme.min.css', 'slick/1.9.0/slick-theme.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/magnific-popup.js/1.2.0/jquery.magnific-popup.min.js', 'magnific-popup/1.2.0/jquery.magnific-popup.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/magnific-popup.js/1.2.0/magnific-popup.min.css', 'magnific-popup/1.2.0/magnific-popup.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.11.1/highlight.min.js', 'highlightjs/11.11.1/highlight.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.11.1/styles/default.min.css', 'highlightjs/11.11.1/default.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/mathjax/3.2.2/es5/tex-chtml.js', 'mathjax/3.2.2/tex-chtml.js'),
        @('https://cdn.jsdelivr.net/npm/chart.js@4.5.0/dist/chart.umd.min.js', 'chartjs/4.5.0/chart.min.js'),
        @('https://cdn.jsdelivr.net/npm/swiper@9.4.1/swiper-bundle.min.js', 'swiper/9.4.1/swiper.min.js'),
        @('https://cdn.jsdelivr.net/npm/swiper@9.4.1/swiper-bundle.min.css', 'swiper/9.4.1/swiper.min.css'),
        @('https://unpkg.com/@webcomponents/webcomponentsjs@2.8.0/webcomponents-loader.js', 'polymer/2.8.0/webcomponents-loader.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/js-cookie/2.2.1/js.cookie.min.js', 'jscookie/2.2.1/js.cookie.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/11.0.2/bootstrap-slider.min.js', 'bootstrap-slider/11.0.2/bootstrap-slider.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/11.0.2/css/bootstrap-slider.min.css', 'bootstrap-slider/11.0.2/bootstrap-slider.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/animate.css/4.1.1/animate.min.css', 'animatecss/4.1.1/animate.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/toastr.js/2.1.4/toastr.min.js', 'toastr/2.1.4/toastr.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/toastr.js/2.1.4/toastr.min.css', 'toastr/2.1.4/toastr.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/plyr/3.8.3/plyr.min.js', 'plyr/3.8.3/plyr.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/plyr/3.8.3/plyr.min.css', 'plyr/3.8.3/plyr.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/rickshaw/1.7.1/rickshaw.min.js', 'rickshaw/1.7.1/rickshaw.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/rickshaw/1.7.1/rickshaw.min.css', 'rickshaw/1.7.1/rickshaw.min.css'),
        @('https://cdnjs.cloudflare.com/ajax/libs/wow/1.1.2/wow.min.js', 'wow/1.1.2/wow.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/jeditable.js/2.0.19/jquery.jeditable.min.js', 'jeditable/2.0.19/jquery.jeditable.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/jquery-validate/1.21.0/jquery.validate.min.js', 'jqueryvalidate/1.21.0/jquery.validate.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/lazysizes/5.3.2/lazysizes.min.js', 'lazysizes/5.3.2/lazysizes.min.js'),
        @('https://cdnjs.cloudflare.com/ajax/libs/clipboard.js/2.0.11/clipboard.min.js', 'clipboard/2.0.11/clipboard.min.js'),
        @('https://cdn.jsdelivr.net/npm/p2p-media-loader-core@0.6.2/build/p2p-media-loader-core.min.js', 'p2pmediacore/0.6.2/p2p-media-loader-core.min.js')
    )

    foreach ($download in $downloads) {
        Invoke-Download $download[0] $download[1]
    }
    Write-Output "Downloaded $($downloads.Count) resource files."
}

if (-not $SkipDownload) {
    Update-Downloads
}

function New-Resource {
    param(
        [string]$File,
        [string]$Library,
        [string]$Version,
        [string]$VersionRange,
        [string]$ContentType,
        [string[]]$Patterns
    )

    $path = Join-Path $root ($File -replace '/', '\')
    $bytes = [System.IO.File]::ReadAllBytes((Resolve-Path $path))
    $sha = [System.Security.Cryptography.SHA384]::Create()
    $sri = 'sha384-' + [Convert]::ToBase64String($sha.ComputeHash($bytes))

    return [PSCustomObject]@{
        id           = "$Library-$Version"
        library      = $Library
        version      = $Version
        versionRange = $VersionRange
        patterns     = $Patterns
        file         = $File
        contentType  = $ContentType
        sri          = $sri
    }
}

function New-Library {
    param(
        [string]$Key,
        [string]$Name,
        [string]$License,
        [object[]]$Resources
    )

    return [PSCustomObject]@{
        key              = $Key
        name             = $Name
        license          = $License
        enabledByDefault = $true
        resources        = @($Resources)
    }
}

function New-Jquery {
    $patterns = @(
        'https://ajax.googleapis.com/ajax/libs/jquery/{version}/jquery.min.js',
        'https://ajax.googleapis.com/ajax/libs/jquery/{version}/jquery.js',
        'https://code.jquery.com/jquery-{version}.min.js',
        'https://code.jquery.com/jquery-{version}.js',
        'https://cdnjs.cloudflare.com/ajax/libs/jquery/{version}/jquery.min.js',
        'https://cdnjs.cloudflare.com/ajax/libs/jquery/{version}/jquery.js',
        'https://cdn.jsdelivr.net/npm/jquery@{version}/dist/jquery.min.js',
        'https://cdn.jsdelivr.net/npm/jquery@{version}/dist/jquery.js',
        'https://unpkg.com/jquery@{version}/dist/jquery.min.js',
        'https://unpkg.com/jquery@{version}/dist/jquery.js',
        'https://yastatic.net/jquery/{version}/jquery.min.js',
        'https://yastatic.net/jquery/{version}/jquery.js'
    )
    return @(
        (New-Resource 'resources/jquery/1.11.3/jquery.min.js' 'jquery' '1.11.3' '>=1.0.0 <2.0.0' $js $patterns),
        (New-Resource 'resources/jquery/2.2.4/jquery.min.js' 'jquery' '2.2.4' '>=2.0.0 <3.0.0' $js $patterns),
        (New-Resource 'resources/jquery/3.7.1/jquery.min.js' 'jquery' '3.7.1' '>=3.0.0 <4.0.0' $js $patterns)
    )
}

function New-BootstrapResources {
    param([string]$Version, [string]$VersionRange)

    $cssPatterns = @(
        "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$Version/css/bootstrap.min.css",
        "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$Version/css/bootstrap.css",
        "https://cdn.jsdelivr.net/npm/bootstrap@$Version/dist/css/bootstrap.min.css",
        "https://cdn.jsdelivr.net/npm/bootstrap@$Version/dist/css/bootstrap.css",
        "https://unpkg.com/bootstrap@$Version/dist/css/bootstrap.min.css",
        "https://unpkg.com/bootstrap@$Version/dist/css/bootstrap.css",
        "https://maxcdn.bootstrapcdn.com/bootstrap/$Version/css/bootstrap.min.css",
        "https://stackpath.bootstrapcdn.com/bootstrap/$Version/css/bootstrap.min.css",
        "https://netdna.bootstrapcdn.com/bootstrap/$Version/css/bootstrap.min.css"
    )
    $jsPatterns = @(
        "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$Version/js/bootstrap.min.js",
        "https://cdnjs.cloudflare.com/ajax/libs/twitter-bootstrap/$Version/js/bootstrap.js",
        "https://cdn.jsdelivr.net/npm/bootstrap@$Version/dist/js/bootstrap.min.js",
        "https://cdn.jsdelivr.net/npm/bootstrap@$Version/dist/js/bootstrap.js",
        "https://unpkg.com/bootstrap@$Version/dist/js/bootstrap.min.js",
        "https://unpkg.com/bootstrap@$Version/dist/js/bootstrap.js",
        "https://maxcdn.bootstrapcdn.com/bootstrap/$Version/js/bootstrap.min.js",
        "https://stackpath.bootstrapcdn.com/bootstrap/$Version/js/bootstrap.min.js",
        "https://netdna.bootstrapcdn.com/bootstrap/$Version/js/bootstrap.min.js"
    )
    $cssPatterns = @($cssPatterns | ForEach-Object { $_.Replace($Version, '{version}') })
    $jsPatterns = @($jsPatterns | ForEach-Object { $_.Replace($Version, '{version}') })

    return @(
        (New-Resource "resources/bootstrap/$Version/bootstrap.min.css" 'bootstrap' $Version $VersionRange $css $cssPatterns),
        (New-Resource "resources/bootstrap/$Version/bootstrap.min.js" 'bootstrap' $Version $VersionRange $js $jsPatterns)
    )
}

function New-Bootstrap {
    $resources = @()
    $resources += New-BootstrapResources '3.4.1' '>=3.0.0 <4.0.0'
    $resources += New-BootstrapResources '4.6.2' '>=4.0.0 <5.0.0'
    $resources += New-BootstrapResources '5.3.8' '>=5.0.0 <6.0.0'
    return $resources
}

function New-FontAwesomeResources {
    param([string]$Version, [string]$VersionRange)

    $major = [version]::new($Version).Major
    if ($major -ge 5) {
        $out = @(
            (New-Resource "resources/fontawesome/$Version/css/all.min.css" 'fontawesome' $Version $VersionRange $css @(
                "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/{version}/css/all.min.css",
                "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@{version}/css/all.min.css",
                "https://unpkg.com/@fortawesome/fontawesome-free@{version}/css/all.min.css",
                "https://use.fontawesome.com/releases/v{version}/css/all.css")),
            (New-Resource "resources/fontawesome/$Version/css/v4-shims.min.css" 'fontawesome' $Version $VersionRange $css @(
                "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/{version}/css/v4-shims.min.css",
                "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@{version}/css/v4-shims.min.css",
                "https://use.fontawesome.com/releases/v{version}/css/v4-shims.css"))
        )
        foreach ($name in @('fa-brands-400', 'fa-regular-400', 'fa-solid-900')) {
            $out += New-Resource "resources/fontawesome/$Version/webfonts/$name.woff2" 'fontawesome' $Version $VersionRange $woff @(
                "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/{version}/webfonts/$name.woff2",
                "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@{version}/webfonts/$name.woff2",
                "https://use.fontawesome.com/releases/v{version}/webfonts/$name.woff2")
        }
        return $out
    }

    return @(
        (New-Resource "resources/fontawesome/$Version/css/font-awesome.min.css" 'fontawesome' $Version $VersionRange $css @(
            "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/{version}/css/font-awesome.min.css",
            "https://cdn.jsdelivr.net/npm/font-awesome@{version}/css/font-awesome.min.css",
            "https://maxcdn.bootstrapcdn.com/font-awesome/{version}/css/font-awesome.min.css",
            "https://netdna.bootstrapcdn.com/font-awesome/{version}/css/font-awesome.min.css")),
        (New-Resource "resources/fontawesome/$Version/fonts/fontawesome-webfont.woff2" 'fontawesome' $Version $VersionRange $woff @(
            "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/{version}/fonts/fontawesome-webfont.woff2",
            "https://cdn.jsdelivr.net/npm/font-awesome@{version}/fonts/fontawesome-webfont.woff2",
            "https://netdna.bootstrapcdn.com/font-awesome/{version}/fonts/fontawesome-webfont.woff2"))
    )
}

function New-FontAwesome {
    $resources = @()
    $resources += New-FontAwesomeResources '4.7.0' '>=4.0.0 <5.0.0'
    $resources += New-FontAwesomeResources '5.15.4' '>=5.0.0 <6.0.0'
    $resources += New-FontAwesomeResources '6.7.2' '>=6.0.0 <7.0.0'
    return $resources
}

function New-MaterialIcons {
    return @(
        (New-Resource 'resources/google-material-icons/v145/material-icons.css' 'googlematerialicons' '1.0.0' '>=1.0.0' $css @(
            'https://fonts.googleapis.com/css?family=Material+Icons',
            'https://fonts.googleapis.com/icon?family=Material+Icons',
            'https://cdnjs.cloudflare.com/ajax/libs/material-design-icons/{version}/iconfont/material-icons.css')),
        (New-Resource 'resources/google-material-icons/v145/MaterialIcons-Regular.woff2' 'googlematerialicons' '1.0.0' '>=1.0.0' $woff @(
            'https://fonts.gstatic.com/s/materialicons/*/MaterialIcons-Regular.woff2',
            'https://cdnjs.cloudflare.com/ajax/libs/material-design-icons/{version}/iconfont/MaterialIcons-Regular.woff2'))
    )
}

function New-React {
    return @(
        (New-Resource 'resources/react/18.3.1/react.production.min.js' 'react' '18.3.1' '>=16.0.0 <20.0.0' $js @(
            'https://unpkg.com/react@{version}/umd/react.production.min.js',
            'https://cdn.jsdelivr.net/npm/react@{version}/umd/react.production.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/react/{version}/umd/react.production.min.js')),
        (New-Resource 'resources/react/18.3.1/react-dom.production.min.js' 'react' '18.3.1' '>=16.0.0 <20.0.0' $js @(
            'https://unpkg.com/react-dom@{version}/umd/react-dom.production.min.js',
            'https://cdn.jsdelivr.net/npm/react-dom@{version}/umd/react-dom.production.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/react-dom/{version}/umd/react-dom.production.min.js'))
    )
}

function New-Vue {
    return @(
        (New-Resource 'resources/vue/2.6.14/vue.min.js' 'vue' '2.6.14' '>=2.0.0 <3.0.0' $js @(
            'https://unpkg.com/vue@{version}/dist/vue.min.js',
            'https://unpkg.com/vue@{version}/dist/vue.js',
            'https://cdn.jsdelivr.net/npm/vue@{version}/dist/vue.min.js',
            'https://cdn.jsdelivr.net/npm/vue@{version}/dist/vue.js',
            'https://cdnjs.cloudflare.com/ajax/libs/vue/{version}/vue.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/vue/{version}/vue.js')),
        (New-Resource 'resources/vue/3.5.22/vue.min.js' 'vue' '3.5.22' '>=3.0.0 <4.0.0' $js @(
            'https://unpkg.com/vue@{version}/dist/vue.global.prod.js',
            'https://unpkg.com/vue@{version}/dist/vue.global.js',
            'https://cdn.jsdelivr.net/npm/vue@{version}/dist/vue.global.prod.js',
            'https://cdn.jsdelivr.net/npm/vue@{version}/dist/vue.global.js',
            'https://cdnjs.cloudflare.com/ajax/libs/vue/{version}/vue.global.prod.js',
            'https://cdnjs.cloudflare.com/ajax/libs/vue/{version}/vue.global.js'))
    )
}

function New-Axios {
    return @(
        (New-Resource 'resources/axios/1.11.0/axios.min.js' 'axios' '1.11.0' '>=0.21.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/axios/{version}/axios.min.js',
            'https://cdn.jsdelivr.net/npm/axios@{version}/dist/axios.min.js',
            'https://unpkg.com/axios@{version}/dist/axios.min.js'))
    )
}

function New-Lodash {
    return @(
        (New-Resource 'resources/lodash/4.17.21/lodash.min.js' 'lodash' '4.17.21' '>=4.0.0 <5.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/lodash.js/{version}/lodash.min.js',
            'https://cdn.jsdelivr.net/npm/lodash@{version}/lodash.min.js',
            'https://unpkg.com/lodash@{version}/lodash.min.js'))
    )
}

function New-Moment {
    return @(
        (New-Resource 'resources/moment/2.30.1/moment-with-locales.min.js' 'moment' '2.30.1' '>=2.10.0 <3.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/moment.js/{version}/moment.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/moment.js/{version}/moment-with-locales.min.js',
            'https://cdn.jsdelivr.net/npm/moment@{version}/moment.min.js',
            'https://unpkg.com/moment@{version}/moment.min.js'))
    )
}

function New-D3 {
    return @(
        (New-Resource 'resources/d3/7.9.0/d3.min.js' 'd3' '7.9.0' '>=6.0.0 <8.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/d3/{version}/d3.min.js',
            'https://cdn.jsdelivr.net/npm/d3@{version}/dist/d3.min.js',
            'https://unpkg.com/d3@{version}/dist/d3.min.js'))
    )
}

function New-Popper {
    return @(
        (New-Resource 'resources/popper/1.16.1/popper.min.js' 'popper' '1.16.1' '>=1.12.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/popper.js/{version}/umd/popper.min.js',
            'https://cdn.jsdelivr.net/npm/popper.js@{version}/dist/umd/popper.min.js',
            'https://unpkg.com/popper.js@{version}/dist/umd/popper.min.js')),
        (New-Resource 'resources/popper/2.11.8/popper.min.js' 'popper' '2.11.8' '>=2.0.0 <3.0.0' $js @(
            'https://cdn.jsdelivr.net/npm/@popperjs/core@{version}/dist/umd/popper.min.js',
            'https://unpkg.com/@popperjs/core@{version}/dist/umd/popper.min.js'))
    )
}

function New-WebFont {
    return @(
        (New-Resource 'resources/webfont/1.6.28/webfontloader.js' 'webfont' '1.6.28' '>=1.5.0 <2.0.0' $js @(
            'https://ajax.googleapis.com/ajax/libs/webfont/{version}/webfont.js',
            'https://cdn.jsdelivr.net/npm/webfontloader@{version}/webfontloader.js',
            'https://unpkg.com/webfontloader@{version}/webfontloader.js'))
    )
}

function New-Angular {
    return @(
        (New-Resource 'resources/angular/1.8.2/angular.min.js' 'angular' '1.8.2' '>=1.0.0 <2.0.0' $js @(
            'https://ajax.googleapis.com/ajax/libs/angularjs/{version}/angular.min.js',
            'https://ajax.googleapis.com/ajax/libs/angularjs/{version}/angular.js',
            'https://cdnjs.cloudflare.com/ajax/libs/angular.js/{version}/angular.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/angular.js/{version}/angular.js',
            'https://cdn.jsdelivr.net/npm/angular@{version}/angular.min.js',
            'https://unpkg.com/angular@{version}/angular.min.js'))
    )
}

function New-Slick {
    return @(
        (New-Resource 'resources/slick/1.9.0/slick.min.js' 'slick' '1.9.0' '>=1.5.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/{version}/slick.min.js',
            'https://cdn.jsdelivr.net/npm/slick-carousel@{version}/slick/slick.min.js',
            'https://unpkg.com/slick-carousel@{version}/slick/slick.min.js')),
        (New-Resource 'resources/slick/1.9.0/slick.min.css' 'slick' '1.9.0' '>=1.5.0 <2.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/{version}/slick.css',
            'https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/{version}/slick.min.css',
            'https://cdn.jsdelivr.net/npm/slick-carousel@{version}/slick/slick.css',
            'https://unpkg.com/slick-carousel@{version}/slick/slick.css')),
        (New-Resource 'resources/slick/1.9.0/slick-theme.min.css' 'slick' '1.9.0' '>=1.5.0 <2.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/{version}/slick-theme.css',
            'https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/{version}/slick-theme.min.css',
            'https://cdn.jsdelivr.net/npm/slick-carousel@{version}/slick/slick-theme.css',
            'https://unpkg.com/slick-carousel@{version}/slick/slick-theme.css'))
    )
}

function New-MagnificPopup {
    return @(
        (New-Resource 'resources/magnific-popup/1.2.0/jquery.magnific-popup.min.js' 'magnificpopup' '1.2.0' '>=1.0.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/magnific-popup.js/{version}/jquery.magnific-popup.min.js',
            'https://cdn.jsdelivr.net/npm/magnific-popup@{version}/dist/jquery.magnific-popup.min.js',
            'https://unpkg.com/magnific-popup@{version}/dist/jquery.magnific-popup.min.js')),
        (New-Resource 'resources/magnific-popup/1.2.0/magnific-popup.min.css' 'magnificpopup' '1.2.0' '>=1.0.0 <2.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/magnific-popup.js/{version}/magnific-popup.min.css',
            'https://cdn.jsdelivr.net/npm/magnific-popup@{version}/dist/magnific-popup.css',
            'https://unpkg.com/magnific-popup@{version}/dist/magnific-popup.css'))
    )
}

function New-HighlightJs {
    return @(
        (New-Resource 'resources/highlightjs/11.11.1/highlight.min.js' 'highlightjs' '11.11.1' '>=9.0.0 <12.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/highlight.js/{version}/highlight.min.js',
            'https://cdn.jsdelivr.net/npm/@highlightjs/cdn-assets@{version}/highlight.min.js',
            'https://unpkg.com/@highlightjs/cdn-assets@{version}/highlight.min.js')),
        (New-Resource 'resources/highlightjs/11.11.1/default.min.css' 'highlightjs' '11.11.1' '>=9.0.0 <12.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/highlight.js/{version}/styles/default.min.css',
            'https://cdn.jsdelivr.net/npm/@highlightjs/cdn-assets@{version}/styles/default.min.css',
            'https://unpkg.com/@highlightjs/cdn-assets@{version}/styles/default.min.css'))
    )
}

function New-MathJax {
    return @(
        (New-Resource 'resources/mathjax/3.2.2/tex-chtml.js' 'mathjax' '3.2.2' '>=3.0.0 <4.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/mathjax/{version}/es5/tex-chtml.js',
            'https://cdn.jsdelivr.net/npm/mathjax@{version}/es5/tex-chtml.js',
            'https://unpkg.com/mathjax@{version}/es5/tex-chtml.js'))
    )
}

function New-ChartJs {
    return @(
        (New-Resource 'resources/chartjs/4.5.0/chart.min.js' 'chartjs' '4.5.0' '>=3.0.0 <5.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/Chart.js/{version}/chart.min.js',
            'https://cdn.jsdelivr.net/npm/chart.js@{version}/dist/chart.umd.min.js',
            'https://unpkg.com/chart.js@{version}/dist/chart.umd.min.js'))
    )
}

function New-Swiper {
    return @(
        (New-Resource 'resources/swiper/9.4.1/swiper.min.js' 'swiper' '9.4.1' '>=6.0.0 <10.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/Swiper/{version}/js/swiper.min.js',
            'https://cdn.jsdelivr.net/npm/swiper@{version}/swiper-bundle.min.js',
            'https://unpkg.com/swiper@{version}/swiper-bundle.min.js')),
        (New-Resource 'resources/swiper/9.4.1/swiper.min.css' 'swiper' '9.4.1' '>=6.0.0 <10.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/Swiper/{version}/css/swiper.min.css',
            'https://cdn.jsdelivr.net/npm/swiper@{version}/swiper-bundle.min.css',
            'https://unpkg.com/swiper@{version}/swiper-bundle.min.css'))
    )
}

function New-Polymer {
    return @(
        (New-Resource 'resources/polymer/2.8.0/webcomponents-loader.js' 'polymer' '2.8.0' '>=2.0.0 <3.0.0' $js @(
            'https://cdn.jsdelivr.net/npm/@webcomponents/webcomponentsjs@{version}/webcomponents-loader.js',
            'https://unpkg.com/@webcomponents/webcomponentsjs@{version}/webcomponents-loader.js',
            'https://cdnjs.cloudflare.com/ajax/libs/webcomponentsjs/{version}/webcomponents-loader.js'))
    )
}

function New-JsCookie {
    return @(
        (New-Resource 'resources/jscookie/2.2.1/js.cookie.min.js' 'jscookie' '2.2.1' '>=2.0.0 <3.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/js-cookie/{version}/js.cookie.min.js',
            'https://cdn.jsdelivr.net/npm/js-cookie@{version}/dist/js.cookie.min.js'))
    )
}

function New-BootstrapSlider {
    return @(
        (New-Resource 'resources/bootstrap-slider/11.0.2/bootstrap-slider.min.js' 'bootstrapslider' '11.0.2' '>=10.0.0 <12.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/{version}/bootstrap-slider.min.js')),
        (New-Resource 'resources/bootstrap-slider/11.0.2/bootstrap-slider.min.css' 'bootstrapslider' '11.0.2' '>=10.0.0 <12.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/bootstrap-slider/{version}/css/bootstrap-slider.min.css'))
    )
}

function New-AnimateCss {
    return @(
        (New-Resource 'resources/animatecss/4.1.1/animate.min.css' 'animatecss' '4.1.1' '>=4.0.0 <5.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/animate.css/{version}/animate.min.css',
            'https://cdn.jsdelivr.net/npm/animate.css@{version}/animate.min.css'))
    )
}

function New-Toastr {
    return @(
        (New-Resource 'resources/toastr/2.1.4/toastr.min.js' 'toastr' '2.1.4' '>=2.0.0 <3.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/toastr.js/{version}/toastr.min.js')),
        (New-Resource 'resources/toastr/2.1.4/toastr.min.css' 'toastr' '2.1.4' '>=2.0.0 <3.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/toastr.js/{version}/toastr.min.css'))
    )
}

function New-Plyr {
    return @(
        (New-Resource 'resources/plyr/3.8.3/plyr.min.js' 'plyr' '3.8.3' '>=3.7.0 <4.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/plyr/{version}/plyr.min.js')),
        (New-Resource 'resources/plyr/3.8.3/plyr.min.css' 'plyr' '3.8.3' '>=3.7.0 <4.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/plyr/{version}/plyr.min.css'))
    )
}

function New-Rickshaw {
    return @(
        (New-Resource 'resources/rickshaw/1.7.1/rickshaw.min.js' 'rickshaw' '1.7.1' '>=1.0.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/rickshaw/{version}/rickshaw.min.js')),
        (New-Resource 'resources/rickshaw/1.7.1/rickshaw.min.css' 'rickshaw' '1.7.1' '>=1.0.0 <2.0.0' $css @(
            'https://cdnjs.cloudflare.com/ajax/libs/rickshaw/{version}/rickshaw.min.css'))
    )
}

function New-Wow {
    return @(
        (New-Resource 'resources/wow/1.1.2/wow.min.js' 'wow' '1.1.2' '>=1.0.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/wow/{version}/wow.min.js'))
    )
}

function New-Jeditable {
    return @(
        (New-Resource 'resources/jeditable/2.0.19/jquery.jeditable.min.js' 'jeditable' '2.0.19' '>=1.0.0 <3.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/jeditable.js/{version}/jquery.jeditable.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/jeditable.js/{version}/jeditable.min.js'))
    )
}

function New-JqueryValidate {
    return @(
        (New-Resource 'resources/jqueryvalidate/1.21.0/jquery.validate.min.js' 'jqueryvalidate' '1.21.0' '>=1.10.0 <2.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/jquery-validate/{version}/jquery.validate.min.js',
            'https://cdnjs.cloudflare.com/ajax/libs/jquery.validate/{version}/jquery.validate.min.js',
            'https://cdn.jsdelivr.net/npm/jquery-validation@{version}/dist/jquery.validate.min.js'))
    )
}

function New-Lazysizes {
    return @(
        (New-Resource 'resources/lazysizes/5.3.2/lazysizes.min.js' 'lazysizes' '5.3.2' '>=5.0.0 <6.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/lazysizes/{version}/lazysizes.min.js'))
    )
}

function New-Clipboard {
    return @(
        (New-Resource 'resources/clipboard/2.0.11/clipboard.min.js' 'clipboard' '2.0.11' '>=2.0.0 <3.0.0' $js @(
            'https://cdnjs.cloudflare.com/ajax/libs/clipboard.js/{version}/clipboard.min.js',
            'https://cdn.jsdelivr.net/npm/clipboard@{version}/dist/clipboard.min.js'))
    )
}

function New-P2pMediaCore {
    return @(
        (New-Resource 'resources/p2pmediacore/0.6.2/p2p-media-loader-core.min.js' 'p2pmediacore' '0.6.2' '>=0.5.0 <1.0.0' $js @(
            'https://cdn.jsdelivr.net/npm/p2p-media-loader-core@{version}/build/p2p-media-loader-core.min.js'))
    )
}

$libraries = @(
    (New-Library 'jquery' 'jQuery' 'MIT' (New-Jquery)),
    (New-Library 'bootstrap' 'Bootstrap' 'MIT' (New-Bootstrap)),
    (New-Library 'fontawesome' 'Font Awesome' 'Font Awesome Free License' (New-FontAwesome)),
    (New-Library 'googlematerialicons' 'Google Material Icons' 'Apache-2.0' (New-MaterialIcons)),
    (New-Library 'react' 'React' 'MIT' (New-React)),
    (New-Library 'vue' 'Vue.js' 'MIT' (New-Vue)),
    (New-Library 'axios' 'Axios' 'MIT' (New-Axios)),
    (New-Library 'lodash' 'Lodash' 'MIT' (New-Lodash)),
    (New-Library 'moment' 'Moment.js' 'MIT' (New-Moment)),
    (New-Library 'd3' 'D3.js' 'ISC' (New-D3)),
    (New-Library 'popper' 'Popper.js' 'MIT' (New-Popper)),
    (New-Library 'webfont' 'Web Font Loader' 'Apache-2.0' (New-WebFont)),
    (New-Library 'angular' 'AngularJS' 'MIT' (New-Angular)),
    (New-Library 'slick' 'Slick Carousel' 'MIT' (New-Slick)),
    (New-Library 'magnificpopup' 'Magnific Popup' 'MIT' (New-MagnificPopup)),
    (New-Library 'highlightjs' 'Highlight.js' 'BSD-3-Clause' (New-HighlightJs)),
    (New-Library 'mathjax' 'MathJax' 'Apache-2.0' (New-MathJax)),
    (New-Library 'chartjs' 'Chart.js' 'MIT' (New-ChartJs)),
    (New-Library 'swiper' 'Swiper' 'MIT' (New-Swiper)),
    (New-Library 'polymer' 'Polymer (Web Components polyfills)' 'BSD-3-Clause' (New-Polymer)),
    (New-Library 'jscookie' 'JavaScript Cookie' 'MIT' (New-JsCookie)),
    (New-Library 'bootstrapslider' 'Bootstrap Slider' 'MIT' (New-BootstrapSlider)),
    (New-Library 'animatecss' 'Animate.css' 'MIT' (New-AnimateCss)),
    (New-Library 'toastr' 'Toastr' 'MIT' (New-Toastr)),
    (New-Library 'plyr' 'Plyr' 'MIT' (New-Plyr)),
    (New-Library 'rickshaw' 'Rickshaw' 'MIT' (New-Rickshaw)),
    (New-Library 'wow' 'WOW.js' 'MIT' (New-Wow)),
    (New-Library 'jeditable' 'jQuery jeditable' 'MIT' (New-Jeditable)),
    (New-Library 'jqueryvalidate' 'jQuery Validation' 'MIT' (New-JqueryValidate)),
    (New-Library 'lazysizes' 'lazysizes' 'MIT' (New-Lazysizes)),
    (New-Library 'clipboard' 'clipboard.js' 'MIT' (New-Clipboard)),
    (New-Library 'p2pmediacore' 'P2P Media Loader Core' 'Apache-2.0' (New-P2pMediaCore))
)

$cdnHosts = @(
    'ajax.googleapis.com', 'code.jquery.com', 'cdnjs.cloudflare.com', 'cdn.jsdelivr.net', 'unpkg.com',
    'maxcdn.bootstrapcdn.com', 'stackpath.bootstrapcdn.com', 'netdna.bootstrapcdn.com', 'use.fontawesome.com',
    'fonts.googleapis.com', 'fonts.gstatic.com', 'yastatic.net', 'yandex.st',
    'cdnjs.loli.net', 'ajax.loli.net', 'fonts.loli.net', 'cdn.bootcdn.net', 'jsdelivr.net',
    'fastly.jsdelivr.net', 'gcore.jsdelivr.net', 'cdn.staticfile.org', 'ajax.aspnetcdn.com', 'lib.baomidou.com'
)

$manifest = [PSCustomObject]@{
    version  = 1
    libraries = @($libraries)
    cdnHosts = @($cdnHosts)
}

$json = $manifest | ConvertTo-Json -Depth 12
$manifestPath = Join-Path $root 'resources.json'
[System.IO.File]::WriteAllText($manifestPath, $json, [System.Text.UTF8Encoding]::new($false))
Write-Output "resources.json written: $manifestPath"
