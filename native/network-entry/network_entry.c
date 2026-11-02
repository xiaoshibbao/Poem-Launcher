#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
#include <windows.h>
#include <winhttp.h>
#include <tlhelp32.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <wchar.h>
#include <string.h>

// Process-local import routing. No inline instruction patches, no changes to
// registry/system proxy, and no modifications to the existing proxy DLL.
static HMODULE self_module;
static USHORT old_port, poem_port;
static int (WSAAPI *real_connect)(SOCKET,const struct sockaddr*,int);
static int (WSAAPI *real_wsa_connect)(SOCKET,const struct sockaddr*,int,LPWSABUF,LPWSABUF,LPQOS,LPQOS);
static volatile LONG redirected;
__declspec(dllexport) volatile LONG PoemNetworkReady;

static const struct sockaddr *redirect_address(const struct sockaddr *input,int length,struct sockaddr_in *copy) {
 if (!old_port || !poem_port || !input || length < (int)sizeof(*copy) || input->sa_family!=AF_INET) return input;
 const struct sockaddr_in *v=(const struct sockaddr_in*)input;
 // Only redirect connections to the configured local HTTP system proxy.
 if ((v->sin_addr.S_un.S_addr & 0xff)!=127 || v->sin_port!=old_port) return input;
 *copy=*v; copy->sin_addr.S_un.S_addr=0x0100007f; copy->sin_port=poem_port;
 InterlockedIncrement(&redirected);return (const struct sockaddr*)copy;
}
static int WSAAPI poem_connect(SOCKET socket,const struct sockaddr *address,int length) {
 struct sockaddr_in copy;const struct sockaddr *selected=redirect_address(address,length,&copy);return real_connect(socket,selected,selected==address?length:sizeof(copy));
}
static int WSAAPI poem_wsa_connect(SOCKET socket,const struct sockaddr *address,int length,LPWSABUF caller,LPWSABUF callee,LPQOS a,LPQOS b) {
 struct sockaddr_in copy;const struct sockaddr *selected=redirect_address(address,length,&copy);return real_wsa_connect(socket,selected,selected==address?length:sizeof(copy),caller,callee,a,b);
}
static LPWSTR copy_proxy_text(LPCWSTR text) {
 SIZE_T size=(wcslen(text)+1)*sizeof(WCHAR);LPWSTR result=GlobalAlloc(GPTR,size);if(result)memcpy(result,text,size);return result;
}
static BOOL WINAPI poem_ie_proxy(WINHTTP_CURRENT_USER_IE_PROXY_CONFIG *config) {
 if(!config){SetLastError(ERROR_INVALID_PARAMETER);return FALSE;}
 WCHAR endpoint[64];_snwprintf(endpoint,64,L"127.0.0.1:%u",ntohs(poem_port));
 ZeroMemory(config,sizeof(*config));
 config->lpszProxy=copy_proxy_text(endpoint);config->lpszProxyBypass=copy_proxy_text(L"localhost;127.*;[::1]");
 if(!config->lpszProxy||!config->lpszProxyBypass){if(config->lpszProxy)GlobalFree(config->lpszProxy);if(config->lpszProxyBypass)GlobalFree(config->lpszProxyBypass);ZeroMemory(config,sizeof(*config));SetLastError(ERROR_NOT_ENOUGH_MEMORY);return FALSE;}
 return TRUE;
}
static void patch_module(HMODULE module);
static HMODULE after_load(HMODULE module,DWORD flags) {
 DWORD error=GetLastError();
 if(module && !(flags&(LOAD_LIBRARY_AS_DATAFILE|LOAD_LIBRARY_AS_IMAGE_RESOURCE|DONT_RESOLVE_DLL_REFERENCES))) {
  WCHAR name[MAX_PATH];if(GetModuleFileNameW(module,name,MAX_PATH)) {
   const WCHAR *base=wcsrchr(name,0x5c);base=base?base+1:name;
   if(_wcsnicmp(base,L"Poem.Proxy",10)!=0 && _wcsicmp(base,L"ProxyHook.dll")!=0)patch_module(module);
  }
 }
 SetLastError(error);return module;
}
static HMODULE WINAPI poem_load_w(LPCWSTR path){return after_load(LoadLibraryW(path),0);}
static HMODULE WINAPI poem_load_a(LPCSTR path){return after_load(LoadLibraryA(path),0);}
static HMODULE WINAPI poem_load_ex_w(LPCWSTR path,HANDLE file,DWORD flags){return after_load(LoadLibraryExW(path,file,flags),flags);}
static HMODULE WINAPI poem_load_ex_a(LPCSTR path,HANDLE file,DWORD flags){return after_load(LoadLibraryExA(path,file,flags),flags);}
static FARPROC WINAPI poem_get_proc(HMODULE module,LPCSTR name) {
 FARPROC result=GetProcAddress(module,name);
 if ((uintptr_t)name<=0xffff) return result;
 if(result==(FARPROC)WinHttpGetIEProxyConfigForCurrentUser && !strcmp(name,"WinHttpGetIEProxyConfigForCurrentUser")) return (FARPROC)poem_ie_proxy;
 if(result==(FARPROC)real_connect && !strcmp(name,"connect")) return (FARPROC)poem_connect;
 if(result==(FARPROC)real_wsa_connect && !strcmp(name,"WSAConnect")) return (FARPROC)poem_wsa_connect;
 return result;
}
static void replace_slot(ULONG_PTR *slot,void *replacement) {
 if ((void*)*slot==replacement) return;
 DWORD previous;
 if(VirtualProtect(slot,sizeof(*slot),PAGE_READWRITE,&previous)) {
  InterlockedExchangePointer((void *volatile*)slot,replacement);
  DWORD ignored;VirtualProtect(slot,sizeof(*slot),previous,&ignored);
 }
}
static void patch_module(HMODULE module) {
 if(module==self_module) return;
 unsigned char *base=(unsigned char*)module;
 IMAGE_DOS_HEADER *dos=(IMAGE_DOS_HEADER*)base;
 if(dos->e_magic!=IMAGE_DOS_SIGNATURE || dos->e_lfanew<0 || dos->e_lfanew>0x100000) return;
 IMAGE_NT_HEADERS64 *nt=(IMAGE_NT_HEADERS64*)(base+dos->e_lfanew);
 if(nt->Signature!=IMAGE_NT_SIGNATURE || nt->OptionalHeader.Magic!=IMAGE_NT_OPTIONAL_HDR64_MAGIC) return;
 DWORD image_size=nt->OptionalHeader.SizeOfImage;
 IMAGE_DATA_DIRECTORY directory=nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT];
 if(!directory.VirtualAddress || directory.VirtualAddress>=image_size || directory.Size<sizeof(IMAGE_IMPORT_DESCRIPTOR)) return;
 IMAGE_IMPORT_DESCRIPTOR *entry=(IMAGE_IMPORT_DESCRIPTOR*)(base+directory.VirtualAddress);
 for(DWORD d=0;d+sizeof(*entry)<=directory.Size && entry->Name;d+=sizeof(*entry),entry++) {
  if(!entry->FirstThunk || entry->FirstThunk>=image_size) continue;
  IMAGE_THUNK_DATA64 *slots=(IMAGE_THUNK_DATA64*)(base+entry->FirstThunk);
  for(DWORD n=0;n<16384 && (unsigned char*)(slots+n+1)<=base+image_size && slots[n].u1.Function;n++) {
   ULONG_PTR value=slots[n].u1.Function;
   if(value==(ULONG_PTR)WinHttpGetIEProxyConfigForCurrentUser) replace_slot(&slots[n].u1.Function,poem_ie_proxy);
   else if(value==(ULONG_PTR)real_connect) replace_slot(&slots[n].u1.Function,poem_connect);
   else if(value==(ULONG_PTR)real_wsa_connect) replace_slot(&slots[n].u1.Function,poem_wsa_connect);
   else if(value==(ULONG_PTR)LoadLibraryW) replace_slot(&slots[n].u1.Function,poem_load_w);
   else if(value==(ULONG_PTR)LoadLibraryA) replace_slot(&slots[n].u1.Function,poem_load_a);
   else if(value==(ULONG_PTR)LoadLibraryExW) replace_slot(&slots[n].u1.Function,poem_load_ex_w);
   else if(value==(ULONG_PTR)LoadLibraryExA) replace_slot(&slots[n].u1.Function,poem_load_ex_a);
   else if(value==(ULONG_PTR)GetProcAddress) replace_slot(&slots[n].u1.Function,poem_get_proc);
  }
 }
}
static DWORD WINAPI watch_modules(void *unused) {
 (void)unused;
 // Cache module identities to avoid repeatedly touching protected IAT pages.
 HMODULE seen[2048];unsigned count=0;
 for(;;) {
  HANDLE snapshot=CreateToolhelp32Snapshot(TH32CS_SNAPMODULE,GetCurrentProcessId());
  if(snapshot!=INVALID_HANDLE_VALUE) {
   MODULEENTRY32W entry;entry.dwSize=sizeof(entry);
   if(Module32FirstW(snapshot,&entry)) do {
    BOOL known=FALSE;for(unsigned i=0;i<count;i++) if(seen[i]==entry.hModule) {known=TRUE;break;}
    if(!known && _wcsnicmp(entry.szModule,L"Poem.Proxy",10)!=0 && _wcsicmp(entry.szModule,L"ProxyHook.dll")!=0) {HMODULE pinned=NULL;if(GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS,(LPCWSTR)entry.hModule,&pinned)){patch_module(pinned);FreeLibrary(pinned);if(count<2048)seen[count++]=entry.hModule;}}
   } while(Module32NextW(snapshot,&entry));
   CloseHandle(snapshot);
   InterlockedExchange(&PoemNetworkReady,1);
  }
  Sleep(100);
 }
 return 0;
}
__declspec(dllexport) LONG WINAPI PoemRedirectCount(void) { return InterlockedCompareExchange(&redirected,0,0); }
BOOL WINAPI DllMain(HINSTANCE module,DWORD reason,LPVOID reserved) {
 (void)reserved;
 if(reason!=DLL_PROCESS_ATTACH) return TRUE;
 self_module=module;
 DisableThreadLibraryCalls(module);
 char old_value[12],new_value[12];
 if(!GetEnvironmentVariableA("POEM_SYSTEM_PROXY_PORT",old_value,sizeof(old_value)) || !GetEnvironmentVariableA("POEM_HTTP_PROXY_PORT",new_value,sizeof(new_value))) return TRUE;
 int a=atoi(old_value),b=atoi(new_value);
 if(a<1 || a>65535 || b<1 || b>65535 || a==b) return TRUE;
 old_port=htons((USHORT)a);poem_port=htons((USHORT)b);
 HMODULE sockets=GetModuleHandleW(L"ws2_32.dll");
 real_connect=(void*)GetProcAddress(sockets,"connect");real_wsa_connect=(void*)GetProcAddress(sockets,"WSAConnect");
 if(!real_connect || !real_wsa_connect) return FALSE;
 // Pin this module: imported callbacks must stay alive until process shutdown.
 HMODULE pin;GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS|GET_MODULE_HANDLE_EX_FLAG_PIN,(LPCWSTR)module,&pin);
 HANDLE worker=CreateThread(NULL,0,watch_modules,NULL,0,NULL);
 if(worker) CloseHandle(worker);
 return TRUE;
}
