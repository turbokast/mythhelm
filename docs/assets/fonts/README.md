# Self-hosted font subsets

The docs site loads no font from a third party. It serves the two WOFF2
subsets in this directory from the site origin, beside the SIL Open Font
License 1.1 notice in `OFL.txt`. Both subsets were made with fontTools
4.63.0 (`fontTools.varLib.instancer` to pin the variable-font instance,
then `pyftsubset` to subset and encode WOFF2). Re-running the commands
below from the downloaded source files reproduces the subsets; the
SHA-256 values pin both the sources used and the subsets shipped.

## Archivo Expanded ExtraBold — `archivo-expanded-extrabold.woff2`

- Family as shipped: `Archivo Expanded ExtraBold` (width 125, weight 800
  instance of the Archivo variable font; CSS `font-family:
  "Archivo Expanded ExtraBold"`, weight 800).
- Source: Archivo by Omnibus-Type, upstream
  https://github.com/Omnibus-Type/Archivo, distributed under the SIL Open
  Font License 1.1 through the Google Fonts OFL directory as
  `ofl/archivo/Archivo[wdth,wght].ttf`, Version 2.001.
- Download URL used:
  `https://raw.githubusercontent.com/google/fonts/main/ofl/archivo/Archivo%5Bwdth%2Cwght%5D.ttf`
- Source file: `Archivo[wdth,wght].ttf`, 658596 bytes, SHA-256
  `0e094a7d3c7c4c25cf1310c4b30014f1dae9332220b1c2c88f4fa996f0b05053`.
- Instancing and subsetting commands:

  ```sh
  python3 -m fontTools.varLib.instancer "Archivo[wdth,wght].ttf" wdth=125 wght=800 --update-name-table -o Archivo-Expanded-ExtraBold.ttf
  pyftsubset Archivo-Expanded-ExtraBold.ttf --output-file=archivo-expanded-extrabold.woff2 --flavor=woff2 --unicodes="U+0020-007E" --desubroutinize
  ```

- Subset: `archivo-expanded-extrabold.woff2`, 7672 bytes, SHA-256
  `599e1af4a12dba8b808fed048c2d9ee2f77a90dd6986280a4c6f73b32aac76f9`;
  covers U+0020–U+007E (basic Latin: the wordmark and headings).
- Reserved Font Name: none. The source `OFL.txt` and the font name table
  declare no Reserved Font Name, so the subset keeps the family name.

## JetBrains Mono — `jetbrains-mono.woff2`

- Family as shipped: `JetBrains Mono` Regular (weight 400 instance of
  the JetBrains Mono variable font; CSS `font-family: "JetBrains Mono"`,
  weight 400, with a system monospace fallback).
- Source: JetBrains Mono by JetBrains, upstream
  https://github.com/JetBrains/JetBrainsMono, distributed under the SIL
  Open Font License 1.1 through the Google Fonts OFL directory as
  `ofl/jetbrainsmono/JetBrainsMono[wght].ttf`, Version 2.211.
- Download URL used:
  `https://raw.githubusercontent.com/google/fonts/main/ofl/jetbrainsmono/JetBrainsMono%5Bwght%5D.ttf`
- Source file: `JetBrainsMono[wght].ttf`, 187208 bytes, SHA-256
  `48715a42ec242c21e9f02692891e147d022299a52e48d5e413e1a942193ffeda`.
- Instancing and subsetting commands:

  ```sh
  python3 -m fontTools.varLib.instancer "JetBrainsMono[wght].ttf" wght=400 --update-name-table -o JetBrainsMono-Regular.ttf
  pyftsubset JetBrainsMono-Regular.ttf --output-file=jetbrains-mono.woff2 --flavor=woff2 --unicodes="U+0020-007E,U+00A0-00FF,U+2013-2014,U+2018-201A,U+201C-201E,U+2020-2022,U+2026,U+2190-2193" --desubroutinize
  ```

- Subset: `jetbrains-mono.woff2`, 18796 bytes, SHA-256
  `aac2040b6c1d1935cd3f73a8dd61dfc51fd8d4edd0648398afa62bd2a4118e57`;
  covers basic Latin, Latin-1 supplement and the listed punctuation and
  arrows (code and labels).
- Reserved Font Name: none. The source `OFL.txt` and the font name table
  declare no Reserved Font Name, so the subset keeps the family name.
