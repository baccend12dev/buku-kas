# Buku Kas

Aplikasi desktop Windows untuk meninjau kas masuk dan kas keluar dari database DBF, memilih akun yang digunakan, serta membuat laporan kas bank harian dengan ekspor Excel.

Proyek ini mengembangkan kembali alur aplikasi VB6 menggunakan **Go, Wails, React, dan TypeScript**. Kolom akun laporan dibuat dinamis berdasarkan pilihan pengguna di BANKLIST, sehingga daftar akun laporan tidak perlu ditulis langsung di kode.

> **Status pengembangan:** pembacaan transaksi, pengelolaan akun BANKLIST, laporan harian, dan ekspor `.xlsx` sudah tersedia. Input, edit, dan hapus transaksi kas belum diimplementasikan.

## Fitur

### Kas Masuk dan Kas Keluar

- Menampilkan transaksi berdasarkan tanggal pilihan; tanggal awal mengikuti tanggal lokal komputer.
- Tombol **Hari ini** untuk kembali ke tanggal saat ini.
- Kolom nomor bukti, tanggal, kode akun, jumlah, dan keterangan.
- Keterangan transaksi dibaca dari file memo DBT.
- Record bertanda terhapus diabaikan; transaksi terakhir dalam urutan fisik DBF ditampilkan lebih dahulu.
- Status memuat, data kosong, dan gagal dengan tombol coba lagi.

### Daftar Akun

- Menampilkan akun yang digunakan dari `BANKLIST.DBF`.
- **Tambah Akun** membuka pilihan dari master `DAFACC.DBF`, dilengkapi pencarian kode/nama dan pemilihan beberapa akun sekaligus.
- Akun yang sudah ditambahkan ditandai dan tidak dapat dipilih kembali.
- Identitas akun memakai gabungan `NOAC1`, `NOAC2`, dan `NOAC3`; nol di depan tetap dipertahankan.
- Checkbox **Laporan** menentukan akun yang masuk laporan dengan menyimpan `STATUS = LAPORAN`. Menghilangkan centang mengosongkan STATUS.
- Akun baru tidak otomatis dipilih untuk laporan.

### Laporan Kas Harian

- Mengikuti tanggal pilihan pada aplikasi.
- Header kolom memakai nama dan kode akun BANKLIST yang dicentang **Laporan**.
- Jumlah kolom mengikuti akun terpilih, dengan urutan fisik BANKLIST dari awal ke akhir.
- Menampilkan saldo awal, rincian penerimaan dan pengeluaran, total harian, akumulasi, dan saldo akhir per akun.
- Rincian diurutkan berdasarkan nomor bukti menaik, mengikuti alur VB6.
- Dua akun laporan dengan `NOAC1` sama ditolak agar transaksi tidak dihitung dua kali.

### Ekspor Excel

- Tersedia pada menu **Laporan**, dalam format `.xlsx`.
- Nama awal file: `Laporan_Kas_YYYY-MM-DD.xlsx`.
- Satu sheet **Laporan Kas**, dengan header dinamis, penanda warna untuk ringkasan, dan format nominal dua desimal.
- Kolom CHECK/GIRO dipertahankan sebagai kolom tersembunyi; semua kolom akun terpilih tetap terlihat.
- Jika data laporan berubah sebelum ekspor, pengguna diminta memuat ulang laporan.
- File dibuat melalui file sementara sebelum mengganti tujuan penyimpanan.

## Tech Stack

| Komponen | Teknologi | Fungsi |
| --- | --- | --- |
| Backend | Go 1.26 | Akses DBF/DBT, validasi, perhitungan, dan penyimpanan BANKLIST |
| Desktop | Wails v3 | Menghubungkan Go dengan antarmuka web dan dialog native Windows |
| Frontend | React 18 + TypeScript | Halaman transaksi, pilihan akun, dan tabel laporan |
| Build frontend | Vite 8 | Development server dan build aset frontend |
| Tampilan | CSS | Layout ringkas, tabel dengan scroll, dan tampilan responsif |
| Penyimpanan | DBF dBASE III + DBT | Data transaksi, master akun, akun terpilih, dan memo |
| Ekspor | Excelize v2.11.1 | Pembuatan workbook Excel dari backend Go |
| Perhitungan uang | `math/big.Int` | Perhitungan laporan eksak dalam satuan sen |
| Pengujian | Go `testing` | Pengujian pembacaan, penyimpanan, laporan, dan ekspor |

Komunikasi frontend–backend menggunakan bindings Wails. Nominal dikirim sebagai string desimal agar tidak kehilangan presisi di JavaScript. Aplikasi berjalan menggunakan file lokal tanpa server database terpisah.

## File Data dan Lokasinya

Semua file DBF dan DBT harus berada **langsung di folder yang sama dengan executable**, bukan dalam subfolder `dbf`.

```text
Folder Aplikasi/
├── buku-kas.exe
├── KAS_IN.DBF
├── KAS_IN.DBT
├── KAS_OUT.DBF
├── KAS_OUT.DBT
├── DAFACC.DBF
└── BANKLIST.DBF
```

| File | Isi | Akses aplikasi |
| --- | --- | --- |
| `KAS_IN.DBF` | Transaksi kas masuk | Baca |
| `KAS_IN.DBT` | Keterangan/memo kas masuk | Baca |
| `KAS_OUT.DBF` | Transaksi kas keluar | Baca |
| `KAS_OUT.DBT` | Keterangan/memo kas keluar | Baca |
| `DAFACC.DBF` | Master akun dan saldo dasar laporan | Baca |
| `BANKLIST.DBF` | Akun yang digunakan dan pilihan akun laporan | Baca, tambah akun, ubah STATUS |

Lokasi data mengikuti direktori executable melalui `os.Executable()`. Tidak diperlukan `.env` atau pemetaan path. Aplikasi tidak mencari data di working directory dan tidak menyalin, memindahkan, atau membuat database awal secara otomatis.

Gunakan folder yang dapat ditulis karena perubahan BANKLIST membutuhkan file sementara, backup, dan lock. Data tetap berupa file terpisah, sehingga pembaruan aplikasi cukup mengganti executable.

Folder `dbf/` lokal di source proyek berisi data/referensi pengembangan dan diabaikan oleh `.gitignore`. Untuk menjalankan aplikasi hasil clone, siapkan file data sendiri dengan struktur field yang didukung. Pembaca DBF ini ditujukan untuk layout yang digunakan proyek, bukan pembaca semua varian DBF.

### Backup dan Lock BANKLIST

Saat menambah akun atau mengubah pilihan laporan, aplikasi menggunakan:

- **`BANKLIST.DBF.bak`** — salinan keadaan BANKLIST sebelum perubahan terakhir yang melewati tahap backup; diperbarui pada penyimpanan berikutnya.
- **`BANKLIST.DBF.lock`** — koordinasi penulisan antarproses Buku Kas. File tetap ada setelah penyimpanan, tetapi kunci OS dilepas saat handle ditutup atau proses berhenti.

Jangan menghapus file lock ketika aplikasi berjalan. Jika perlu menghapus file pendukung, tutup semua instans Buku Kas terlebih dahulu. Menghapus `.bak` menghilangkan cadangan terakhir.

## Aturan Perhitungan Laporan

Perhitungan mengikuti alur aplikasi VB6:

```text
Saldo awal = SAWD_00 + kas masuk sebelum tanggal - kas keluar sebelum tanggal
Total penerimaan = kas masuk tepat pada tanggal pilihan
Total pengeluaran = kas keluar tepat pada tanggal pilihan
Akumulasi = total mutasi sampai termasuk tanggal pilihan
Saldo akhir = saldo awal + total penerimaan - total pengeluaran
```

Saldo dasar `SAWD_00` diambil dari DAFACC menggunakan gabungan tiga kode akun. Transaksi dicocokkan ke kolom laporan melalui `KPD1` untuk kas masuk dan `KPK1` untuk kas keluar, berdasarkan `NOAC1`.

Akumulasi menggunakan seluruh riwayat yang tersedia dalam file, tanpa batas bulan atau tahun tambahan. Laporan merupakan posisi kas per akun; selisih kas masuk dan kas keluar tidak otomatis merupakan laba.

## Menjalankan Aplikasi

Untuk pengguna aplikasi yang sudah dibangun:

1. Siapkan **Microsoft Edge WebView2 Runtime** di Windows.
2. Letakkan `buku-kas.exe` bersama semua DBF dan DBT sesuai susunan di atas.
3. Jalankan executable dan pilih tanggal transaksi.
4. Buka **Daftar Akun**. Gunakan **Tambah Akun** untuk memilih akun dari DAFACC jika diperlukan.
5. Centang **Laporan** pada akun yang akan dilaporkan.
6. Buka **Laporan** untuk meninjau hasil dan gunakan **Export Excel** untuk menyimpannya.

Komputer pengguna tidak memerlukan instalasi Go, Node.js, atau Microsoft Excel untuk membuat file `.xlsx`. File hasil ekspor dapat dibuka dengan aplikasi spreadsheet yang kompatibel.

## Pengembangan

### Prasyarat

- Windows sebagai target aplikasi saat ini.
- Go 1.26.0 atau lebih baru.
- Node.js 22.12.0 atau lebih baru beserta npm.
- Wails CLI v3.0.0.
- Microsoft Edge WebView2 Runtime.

### Instalasi Dependency

Jalankan dari root proyek menggunakan PowerShell:

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
go mod download

Set-Location frontend
npm.cmd ci
Set-Location ..

wails3 doctor
```

Pastikan direktori binary Go tersedia di PATH agar perintah `wails3` dikenali. Penggunaan `npm.cmd` menghindari hambatan execution policy PowerShell untuk `npm.ps1`.

Proyek memiliki konfigurasi mirror registry di `frontend/.npmrc`. Konfigurasi tersebut dapat disesuaikan dengan jaringan development; verifikasi sertifikat tetap diaktifkan.

### Development Mode

```powershell
wails3 dev
```

Mode ini menjalankan aplikasi dengan pemuatan ulang perubahan kode. Executable development berada di `bin/`, sehingga DBF dan DBT untuk pengujian harus disiapkan langsung di `bin/`. Gunakan **salinan data pengujian**; data lokal dari `dbf/` tidak dibaca otomatis.

Frontend memanggil service Go melalui Wails. Menjalankan Vite saja tidak menyediakan akses DBF dari backend.

### Build Rilis Windows

```powershell
wails3 build
```

Hasil build berada di:

```text
bin/buku-kas.exe
```

Konfigurasi rilis Windows menggunakan `-trimpath` dan `-ldflags="-w -s -H windowsgui"` untuk membuang informasi debug dan membangun aplikasi GUI. DBF dan DBT tidak ikut dikemas ke executable.

### Pemeriksaan

```powershell
go test ./...
go vet ./...

Set-Location frontend
npm.cmd run build
Set-Location ..
```

Pengujian Go mencakup validasi DBF/DBT, pemilihan lokasi data berdasarkan executable, penambahan akun, STATUS laporan, backup dan penguncian, perhitungan uang, batas tanggal, serta ekspor Excel. Pengujian penulisan menggunakan fixture atau direktori sementara. Beberapa pemeriksaan read-only terhadap data lokal dilewati jika file sumber tidak tersedia.

## Struktur Proyek

```text
buku-kas/
├── main.go                 # Entry point aplikasi Wails
├── cashservice.go          # Pembacaan transaksi kas
├── dbf.go                  # Validasi layout DBF
├── accounts.go             # Pembacaan BANKLIST dan master akun
├── bankaccounts.go         # Penambahan akun dan staging penyimpanan
├── bankfile_*.go           # Penguncian dan penggantian file per platform
├── reportstatus.go         # Penyimpanan pilihan akun laporan
├── cashreport.go           # Perhitungan laporan kas harian
├── reportexport.go         # Pembuatan dan penyimpanan Excel
├── *_test.go               # Pengujian backend
├── frontend/
│   ├── src/                # Komponen React dan TypeScript
│   ├── public/             # CSS, font, dan aset statis
│   └── bindings/           # Bindings Wails hasil generate
├── build/                  # Konfigurasi dan aset build Wails
├── Taskfile.yml            # Task development dan build
└── README.md
```

Bindings di `frontend/bindings/` dibuat oleh Wails dan tidak diedit secara manual. Folder `build/` berisi kebutuhan build, bukan data transaksi.

## Batasan Saat Ini

- Input, edit, dan hapus transaksi kas belum tersedia.
- Belum ada input akun manual, edit identitas akun, hapus/nonaktifkan akun BANKLIST, atau perubahan saldo dasar melalui aplikasi.
- Laporan baru mendukung tanggal harian; laporan bulanan/periode dan accounting lengkap belum tersedia.
- Export Excel hanya tersedia pada menu Laporan.
- Pembacaan teks dibatasi pada encoding yang didukung implementasi saat ini, terutama ASCII.
- Penulisan BANKLIST bersama aplikasi lain yang tidak mengikuti protokol lock Buku Kas belum didukung.
- Pembacaan beberapa DBF belum menjamin snapshot konsisten jika aplikasi lain menulis sumber transaksi bersamaan.
- Target penggunaan saat ini adalah Windows; konfigurasi platform lain dari starter Wails belum menjadi jaminan dukungan.

Repository menyimpan source aplikasi. Data transaksi, backup, file lock, dan laporan hasil ekspor merupakan file operasional lokal dan tidak diperlukan untuk memahami kode aplikasi.
