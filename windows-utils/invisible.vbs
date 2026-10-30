' Launches remixer hidden with -loop.
' Usage: wscript.exe invisible.vbs [path\to\remixer.exe]
Dim shell, exe
Set shell = CreateObject("Wscript.Shell")

exe = "remixer.exe"
If WScript.Arguments.Count > 0 Then exe = WScript.Arguments(0)

shell.Run """" & exe & """ -loop", 0, false
