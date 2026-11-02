#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdio.h>
#include <stdlib.h>
int wmain(int argc,WCHAR **argv){
 if(argc<3)return 2;
 HMODULE dll=LoadLibraryW(argv[1]);if(!dll){printf("load=%lu\n",GetLastError());return 3;}
 void *(*newstate)(void)=(void *)GetProcAddress(dll,"luaL_newstate");
 int (*load)(void *,const char *,size_t,const char *)=(void *)GetProcAddress(dll,"xluaL_loadbuffer");
 void (*close_lua)(void *)=(void *)GetProcAddress(dll,"lua_close");
 const char *(*message)(void *,int,size_t *)=(void *)GetProcAddress(dll,"lua_tolstring");
 void (*openlibs)(void *)=(void *)GetProcAddress(dll,"luaL_openlibs");
 int (*pcall_lua)(void *,int,int,int)=(void *)GetProcAddress(dll,"lua_pcall");
 if(!newstate||!load||!close_lua||!message||!openlibs||!pcall_lua)return 4;
 FILE *f=_wfopen(argv[2],L"rb");if(!f)return 5;
 fseek(f,0,SEEK_END);long size=ftell(f);rewind(f);if(size<0||size>8*1024*1024)return 6;
 char *source=malloc(size+1);if(!source)return 7;
 if(fread(source,1,size,f)!=(size_t)size)return 8;fclose(f);source[size]=0;
 void *L=newstate();if(!L)return 9;
 int result=load(L,source,size,"@Poem/compile-fixture");
 if(!result && argc>3){openlibs(L);result=pcall_lua(L,0,0,0);}
 if(result){size_t length;const char *error=message(L,-1,&length);printf("Lua error: %s\n",error?error:"unknown");}
 else printf("Lua %s OK\n",argc>3?"execution":"syntax");
 close_lua(L);free(source);return result?10:0;
}
