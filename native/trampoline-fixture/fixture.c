#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static LPVOID (WINAPI *os_alloc)(LPVOID,SIZE_T,DWORD,DWORD);
static void *actual, *requested;
static LPVOID WINAPI moved_alloc(LPVOID address,SIZE_T size,DWORD type,DWORD protection) {
 if(type==0x3000 && size==0x1000 && protection==PAGE_EXECUTE_READWRITE) { requested=address;return actual; }
 return os_alloc(address,size,type,protection);
}
int main(int argc,char **argv) {
 if(argc!=3) return 2;
 HMODULE dll=LoadLibraryA(argv[1]);if(!dll){printf("load=%lu\n",GetLastError());return 3;}
 unsigned char *base=(unsigned char*)dll;
 struct { void *previous; void *thread; } frame;
 void (*enter)(void*)=(void*)(base+0xe2c70);
 void (*leave)(void*)=(void*)(base+0xe2dc0);
 if(strcmp(argv[2],"protect")==0) {
  unsigned char *region=VirtualAlloc(NULL,0x2000,MEM_RESERVE,PAGE_NOACCESS);
  if(!region || !VirtualAlloc(region,0x1000,MEM_COMMIT,PAGE_READWRITE))return 7;
  DWORD old;
  if(!VirtualProtect(region,0x1000,PAGE_EXECUTE_READ,&old))return 8;
  void (*protect)(void*,int)=(void*)(base+0xf3a0);
  enter(&frame);
  protect(region+0xca0,PAGE_EXECUTE_READWRITE);
  leave(&frame);
  MEMORY_BASIC_INFORMATION info;
  if(!VirtualQuery(region,&info,sizeof(info)))return 9;
  int writable=info.Protect==PAGE_EXECUTE_READWRITE;
  if(writable)memset(region+0xca0,0xcc,64);
  printf("protectionWritable=%d protection=%lx\n",writable,(unsigned long)info.Protect);
  VirtualFree(region,0,MEM_RELEASE);
  return 0;
 }
 // Exact version is pinned by the launcher tests before running this fixture.
 LPVOID *slot=(LPVOID*)(base+0x1482a0);os_alloc=(void*)*slot;
 actual=os_alloc(NULL,0x1000,0x3000,PAGE_EXECUTE_READWRITE);if(!actual)return 4;
 DWORD previous;if(!VirtualProtect(slot,sizeof(void*),PAGE_READWRITE,&previous))return 5;
 *slot=moved_alloc;
 void *(*allocate)(void*,int)=(void*)(base+strtoul(argv[2],NULL,16));
 enter(&frame);
 void *result=allocate(GetProcAddress(GetModuleHandleA("kernel32.dll"),"GetTickCount"),0x1000);
 leave(&frame);
 *slot=os_alloc;DWORD ignored;VirtualProtect(slot,sizeof(void*),previous,&ignored);
 MEMORY_BASIC_INFORMATION info;ZeroMemory(&info,sizeof(info));VirtualQuery(result,&info,sizeof(info));
 int safe=result==actual && info.State==MEM_COMMIT && (info.Protect&PAGE_EXECUTE_READWRITE) && (uintptr_t)info.BaseAddress+info.RegionSize-(uintptr_t)result>=64;
 if(safe)memset(result,0xcc,64);
 printf("allocator=%s requested=%p allocated=%p returned=%p matches=%d safeWrite64=%d\n",argv[2],requested,actual,result,result==actual,safe);
 VirtualFree(actual,0,MEM_RELEASE);
 return requested?0:6;
}
