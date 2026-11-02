#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
#include <windows.h>
#include <winhttp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
int main(int argc,char **argv) {
 if(argc!=4) return 2;
 WSADATA data;if(WSAStartup(MAKEWORD(2,2),&data))return 3;
 HMODULE module=LoadLibraryA(argv[1]);if(!module){printf("load error=%lu\n",GetLastError());return 4;}
 Sleep(300);
 WINHTTP_CURRENT_USER_IE_PROXY_CONFIG config;if(!WinHttpGetIEProxyConfigForCurrentUser(&config)){printf("IE proxy query failed\n");return 7;}
 WCHAR expected[64];_snwprintf(expected,64,L"127.0.0.1:%u",(unsigned)atoi(getenv("POEM_HTTP_PROXY_PORT")));if(!config.lpszProxy || wcscmp(config.lpszProxy,expected)){printf("wrong process proxy\n");return 8;}GlobalFree(config.lpszProxy);GlobalFree(config.lpszProxyBypass);
 for(int i=0;i<4;i++){
  SOCKET s=socket(AF_INET,SOCK_STREAM,IPPROTO_TCP);struct sockaddr_in address;memset(&address,0,sizeof(address));address.sin_family=AF_INET;address.sin_addr.s_addr=0x0100007f;address.sin_port=htons((unsigned short)atoi(argv[2]));
  int result;
  if(i==0)result=connect(s,(struct sockaddr*)&address,sizeof(address));
  else if(i==1){int(WSAAPI *fn)(SOCKET,const struct sockaddr*,int)=(void*)GetProcAddress(GetModuleHandleA("ws2_32.dll"),"connect");result=fn(s,(struct sockaddr*)&address,sizeof(address));}
  else if(i==2)result=WSAConnect(s,(struct sockaddr*)&address,sizeof(address),NULL,NULL,NULL,NULL);
  else {int(WSAAPI *fn)(SOCKET,const struct sockaddr*,int,LPWSABUF,LPWSABUF,LPQOS,LPQOS)=(void*)GetProcAddress(GetModuleHandleA("ws2_32.dll"),"WSAConnect");result=fn(s,(struct sockaddr*)&address,sizeof(address),NULL,NULL,NULL,NULL);}
  if(result){printf("connect failed mode=%d error=%d\n",i,WSAGetLastError());return 5;}
  const char *request="GET http://globaldp-beta-os01.starrails.com/query_dispatch HTTP/1.1\r\nHost: globaldp-beta-os01.starrails.com\r\nConnection: close\r\n\r\n";
  send(s,request,(int)strlen(request),0);char buffer[8192];int total=0,n;while(total<8191&&(n=recv(s,buffer+total,8191-total,0))>0)total+=n;buffer[total]=0;closesocket(s);
  if(!strstr(buffer,argv[3])){printf("unexpected response mode=%d: %s\n",i,buffer);return 6;}
 }
 LONG(WINAPI *counter)(void)=(void*)GetProcAddress(module,"PoemRedirectCount");printf("redirected=%ld\n",counter());
 // Keep the module pinned for callback lifetime; OS cleans it up on exit.
 return 0;
}
