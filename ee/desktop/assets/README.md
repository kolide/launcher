# Icons

Windows uses an `ICO` style file, some [docs](https://docs.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-notifyicondataa?source=recommendations#nif_showtip-0x00000080) say:
> A handle to the icon to be added, modified, or deleted. Windows XP and later support icons of up to 32 BPP.
>
> If only a 16x16 pixel icon is provided, it is scaled to a larger size in a system set to a high dpi value. This can lead to an unattractive result. It is recommended that you provide both a 16x16 pixel icon and a 32x32 icon in your resource file. Use LoadIconMetric to ensure that the correct icon is loaded and scaled appropriately. See Remarks for a code example.
I think the implication is that windows supports multiple resolution ico files, and it will pick the appropriate values. 

To convert:

```shell
dir=$(mktemp -d)
for p in *png; do
  for s in 16 32 64 128; do
    convert -resize ${s}x${s} "$p" "$dir/$p-$s.ico"
    convert "$dir/$p-*.ico" $(basename $p .png).ico
  done
done
```