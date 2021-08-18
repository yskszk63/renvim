use std::env;
use std::io;
use std::os::unix::net::UnixStream;
use std::os::unix::process::CommandExt;
use std::process::Command;

use rmp::{decode as dec, encode as enc};

fn main() -> anyhow::Result<()> {
    let val = env::var("NVIM_LISTEN_ADDRESS").ok();
    let args = env::args().skip(1).collect::<Vec<_>>();
    let optonly = args.iter().all(|arg| arg.starts_with("--"));

    if val.is_none() || val == Some("".into()) || (!args.is_empty() && optonly) {
        for arg in &args {
            if arg == "--version" {
                println!("renvim v0.0.0 -- Neovim wrapper.\n")
            }
        }
        return Err(Command::new("nvim").args(args).exec().into());
    }

    let mut conn = UnixStream::connect(val.unwrap())?;

    fn tabnew<W>(conn: &mut W, file: Option<String>) -> anyhow::Result<()>
    where
        W: io::Write,
    {
        enc::write_array_len(conn, 4)?;
        enc::write_u8(conn, 0)?;
        enc::write_u32(conn, 0)?;
        enc::write_str(conn, "nvim_command")?;

        let command = file
            .map(|f| format!("tabnew {}", f))
            .unwrap_or("tabnew".into());
        enc::write_array_len(conn, 1)?;
        enc::write_str(conn, &command)?;

        anyhow::Result::<_>::Ok(())
    }

    fn recv<R>(conn: &mut R) -> anyhow::Result<()>
    where
        R: io::Read,
    {
        match dec::read_array_len(conn)? {
            4 => {}
            x => anyhow::bail!("unexpected array len. {}", x),
        }

        dec::read_int::<usize, _>(conn)?;
        dec::read_int::<usize, _>(conn)?;

        match dec::read_array_len(conn)? {
            0 => {}
            x => anyhow::bail!("unexpected array len. {}", x),
        }

        anyhow::Result::<_>::Ok(())
    }

    if args.is_empty() {
        tabnew(&mut conn, None)?;
        recv(&mut conn)?;
    } else {
        for arg in args {
            tabnew(&mut conn, Some(arg))?;
            recv(&mut conn)?;
        }
    }

    Ok(())
}
