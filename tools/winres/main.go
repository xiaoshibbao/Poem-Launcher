package main
import (
 "os"
 "log"
 "github.com/tc-hib/winres"
 "github.com/tc-hib/winres/version"
)
func main(){
 rs:=winres.ResourceSet{}
 f,err:=os.Open("../../build/windows/icon.ico");if err!=nil{log.Fatal(err)}
 ico,err:=winres.LoadICO(f);f.Close();if err!=nil{log.Fatal(err)}
 if err=rs.SetIcon(winres.RT_ICON,ico);err!=nil{log.Fatal(err)}
 manifest,err:=os.ReadFile("../../build/windows/wails.exe.manifest");if err!=nil{log.Fatal(err)}
 parsed,err:=winres.AppManifestFromXML(manifest);if err!=nil{log.Fatal(err)};rs.SetManifest(parsed)
 b,err:=os.ReadFile("../../build/windows/info.json");if err!=nil{log.Fatal(err)}
 var v version.Info;if err=v.UnmarshalJSON(b);err!=nil{log.Fatal(err)};rs.SetVersionInfo(v)
 out,err:=os.Create("../../rsrc_windows_amd64.syso");if err!=nil{log.Fatal(err)};defer out.Close()
 if err=rs.WriteObject(out,winres.ArchAMD64);err!=nil{log.Fatal(err)}
}
