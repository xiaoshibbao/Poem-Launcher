#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdio.h>
typedef int (*PCall)(void *,int,int,int);
static PCall call;
static DWORD WINAPI secondary(void *unused){for(int i=0;i<25;i++){call((void *)0x1234,1,0,0);Sleep(10);}return 0;}
int wmain(int argc,WCHAR **argv){
 if(argc!=4)return 2;
 WNDCLASSW klass={0};klass.lpfnWndProc=DefWindowProcW;klass.hInstance=GetModuleHandleW(NULL);klass.lpszClassName=L"UnityWndClass";
 if(!RegisterClassW(&klass))return 3;
 HWND window=CreateWindowW(klass.lpszClassName,L"Poem isolated Lua fixture",WS_OVERLAPPED,0,0,100,100,NULL,NULL,klass.hInstance,NULL);
 if(!window)return 4;
 HMODULE lua=LoadLibraryW(argv[1]);if(!lua)return 5;
 void (*init)(void)=(void *)GetProcAddress(lua,"PoemFixtureInit");init();
 call=(void *)GetProcAddress(lua,"lua_pcall");
 if(!LoadLibraryW(argv[2]))return 6;
 Sleep(700);
 HANDLE thread=CreateThread(NULL,0,secondary,NULL,0,NULL);WaitForSingleObject(thread,5000);CloseHandle(thread);
 for(int i=0;i<30;i++){call((void *)0x1234,1,0,0);Sleep(10);}
 char report[256];int (*report_fn)(char *,size_t)=(void *)GetProcAddress(lua,"PoemFixtureReport");report_fn(report,sizeof(report));
 HANDLE f=CreateFileW(argv[3],GENERIC_WRITE,0,NULL,CREATE_ALWAYS,FILE_ATTRIBUTE_NORMAL,NULL);if(f==INVALID_HANDLE_VALUE)return 7;
 DWORD written;WriteFile(f,report,(DWORD)strlen(report),&written,NULL);CloseHandle(f);
 DestroyWindow(window);return 0;
}
