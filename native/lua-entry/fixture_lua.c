#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdio.h>
#include <string.h>
#include <stdint.h>
static DWORD owner;
static int top=7,executions,wrong_thread,loading,failed;
__declspec(dllexport) void PoemFixtureInit(void){owner=GetCurrentThreadId();}
__declspec(dllexport) int lua_gettop(void *L){return top;}
__declspec(dllexport) void lua_settop(void *L,int value){top=value;}
__declspec(dllexport) const char *lua_tolstring(void *L,int index,size_t *len){const char *error="fixture Lua runtime error";*len=strlen(error);return error;}
__declspec(dllexport) int xluaL_loadbuffer(void *L,const char *script,size_t size,const char *name){
 if(GetCurrentThreadId()!=owner)wrong_thread++;
 loading=1;failed=strstr(script,"FAIL_LUA")!=NULL;top++;
 return 0;
}
__declspec(dllexport) __attribute__((noinline)) int lua_pcall(void *L,int args,int results,int error_function){
 volatile uintptr_t marker=(uintptr_t)L;marker^=(uintptr_t)args;
 if(loading){loading=0;executions++;if(GetCurrentThreadId()!=owner)wrong_thread++;if(!failed)top--;return failed?2:0;}
 return marker==(uintptr_t)-1?1:0;
}
__declspec(dllexport) int PoemFixtureReport(char *text,size_t size){return snprintf(text,size,"executions=%d wrongThread=%d top=%d",executions,wrong_thread,top);}
BOOL WINAPI DllMain(HINSTANCE module,DWORD reason,LPVOID reserved){return TRUE;}
