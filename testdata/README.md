# Test corpus

Corpus files are organized by generator, document type, feature, and language.
Every file must have a row in `manifest.csv` with its source and license.

## Layout

```text
testdata/
  manifest.csv          # file, generator, type, features, language, source, license
  corpus/               # real-world samples (public or cleared for commit)
  goldens/              # structural and display-list golden outputs
  fonts/                # small licensed font fixtures for metric tests
  fuzz/                 # seed inputs for native fuzz targets
```

## Labels

- **generator**: Word, WPS, PowerPoint, LaTeX, Chrome/Skia, LibreOffice, Acrobat Distiller, ...
- **type**: native, scanned, mixed
- **features**: classic-xref, xref-stream, objstm, incremental, encrypted, filter-flate, ...
- **language**: zh, en, mixed, ja, ko

Font fixtures under `fonts/` are test-only resources with their license files
next to them. They are not part of the PDF corpus manifest.

Confidential or customer documents must stay in the separate internal corpus
repository and must never be committed here.
