//! Standalone Rust C++ DWARF v4 and `.symtab` `_ZTV` seeder (`twdll-dwarf`).
//!
//! Usage: `twdll-dwarf <elf_path> <staging_sqlite_path>`

mod db;
mod dwarf;
mod layout;
mod vtable;

use std::env;
use std::fs::File;
use std::path::Path;
use std::process;

fn run() -> Result<(), String> {
    let args: Vec<String> = env::args().collect();
    if args.len() != 3 {
        return Err(format!(
            "Usage: {} <elf_path> <staging_sqlite_path>",
            args.first().map_or("twdll-dwarf", String::as_str)
        ));
    }

    let elf_path = Path::new(&args[1]);
    let staging_sqlite_path = Path::new(&args[2]);

    let file = File::open(elf_path)
        .map_err(|e| format!("Failed opening ELF {}: {e}", elf_path.display()))?;
    let mmap = unsafe { memmap2::Mmap::map(&file) }
        .map_err(|e| format!("Failed memory-mapping ELF {}: {e}", elf_path.display()))?;

    let batch = dwarf::extract_elf_dwarf(&mmap)?;
    db::write_batch_to_staging_db(staging_sqlite_path, &batch)?;

    Ok(())
}

fn main() {
    if let Err(err) = run() {
        eprintln!("twdll-dwarf error: {err}");
        process::exit(1);
    }
}
