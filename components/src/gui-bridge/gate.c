/* Offline prototype: original ABI is int Action(const char *), not JSON.
 * No shell or authentication state in this hook. The broker must authorize
 * every operation. Absolute I/O deadline bounds main-thread blocking.
 * Linux AArch64 syscalls avoid adding dynamic loader dependencies. */
typedef unsigned long U;
typedef unsigned int W;
struct ts { long sec, ns; };
struct pollfd { int fd; short events, revents; };
extern void owner_notify(const char *);
static long sc(long n,long a,long b,long c,long d,long e,long f) {
 register long x8 __asm__("x8")=n, x0 __asm__("x0")=a, x1 __asm__("x1")=b;
 register long x2 __asm__("x2")=c,x3 __asm__("x3")=d,x4 __asm__("x4")=e,x5 __asm__("x5")=f;
 __asm__ volatile("svc #0" : "+r"(x0) : "r"(x8),"r"(x1),"r"(x2),"r"(x3),"r"(x4),"r"(x5) : "memory","cc");
 return x0;
}
static long now(void) { struct ts t; if(sc(113,1,(long)&t,0,0,0,0))return -1; return t.sec*1000000000L+t.ns; }
static int ready(int fd,int event,long deadline) {
 for(int i=0;i<16;i++) {
  long n=now(); if(n<0 || n>=deadline)return -1;
  long rem=deadline-n; struct ts t={rem/1000000000L,rem%1000000000L};
  struct pollfd p={fd,(short)event,0}; long r=sc(73,(long)&p,1,(long)&t,0,0,0);
  if(r>0) return (p.revents & event)?0:-1;
  if(r!=-4)return -1;
 }return -1;
}
static int transfer(int fd,char *p,U n,int write,long deadline) {
 for(int i=0;i<2048 && n;i++) {
  if(ready(fd,write?4:1,deadline))return -1;
  long r=write?sc(206,fd,(long)p,n,0x4000,0,0):sc(63,fd,(long)p,n,0,0,0);
  if(r==-4 || r==-11)continue;
  if(r<=0 || (U)r>n)return -1;
  p+=r;n-=r;
 }return n?-1:0;
}
#ifndef BRIDGE_HELPERS_ONLY
__attribute__((section(".text.entry"))) int owner_gui_gate(const char *input) {
 int fd=-1; char request[772],response[1024]; W len=0;
 const char *message="OWNER_GUI_DENIED";
 if(!input)goto done;
 while(len<768 && input[len]) {request[4+len]=input[len];len++;}
 if(!len || len==768)goto done;
 for(int i=0;i<4;i++)request[i]=(char)(len>>(i*8));
 long start=now();if(start<0)goto done;long deadline=start+1500000000L;
 fd=sc(198,1,0x80801,0,0,0,0);if(fd<0)goto done;
 const struct {short family;char path[sizeof("/run/x6-owner-gui/auth.sock")];} addr={1,"/run/x6-owner-gui/auth.sock"};
 long r=sc(203,fd,(long)&addr,sizeof(addr),0,0,0);
 if(r && r!=-115 && r!=-11)goto done;
 if(r) {
  if(ready(fd,4,deadline))goto done;
  int error=0;W size=4;
  if(sc(209,fd,1,4,(long)&error,(long)&size,0) || size!=4 || error)goto done;
 }
 struct {int pid;W uid,gid;} peer;W size=12;
 if(sc(209,fd,1,17,(long)&peer,(long)&size,0) || size!=12 || peer.uid!=0)goto done;
 if(transfer(fd,request,len+4,1,deadline))goto done;
 W out=0;if(transfer(fd,(char*)&out,4,0,deadline))goto done;
 if(!out || out>1023)goto done;
 if(transfer(fd,response,out,0,deadline))goto done;
 /* Broker replies are printable protocol records, never printf formats. */
 for(W i=0;i<out;i++)if(response[i]<32 || response[i]>126)goto done;
 response[out]=0;message=response;
done:
 if(fd>=0)sc(57,fd,0,0,0,0,0);
 owner_notify(message);
 return 0;
}

#endif
