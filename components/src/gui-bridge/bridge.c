/* Private GUI bridge. No public listener and no command execution here.
 * Started after the original GUI initialization, never a constructor.
 * Only this dedicated worker invokes Native_Exec; the original shell table,
 * packer, parser and action callback remain in the path. */
#define BRIDGE_HELPERS_ONLY
#include "gate.c"

extern void *dlsym(void *, const char *);
extern int pthread_create(U *, const void *, void *(*)(void *), void *);
extern int pthread_join(U, void **);
extern void *dlopen(const char *, int);
extern int dlclose(void *);
static int (*dispatch)(const char *, const char *, ...);
static void (*vendor_notify)(const char *);
static int started, active_tid, stopping, thread_live;
static U worker_thread;
static void *resolve_notify(void) {
 void *h=dlopen("libins_gui_wrap.so",5); /* RTLD_LAZY | RTLD_NOLOAD: the vendor loads this RTLD_LOCAL. */
 if(!h)return 0;
 void *p=dlsym(h,"InsGui_DeveloperExecuteCommandResult_Notify");dlclose(h);return p;
}
static unsigned count, captured_len;
static char captured[1024];
static const char path[]="/run/x6-owner-gui-bridge/dispatch.sock";
/* Called through the original library's notification PLT. Forward every
 * event to the real vendor wrapper; capture only this worker's current call. */
void InsGui_DeveloperExecuteCommandResult_Notify(const char *s) {
 int tid=(int)sc(178,0,0,0,0,0,0);
 if(tid==__atomic_load_n(&active_tid,__ATOMIC_ACQUIRE)) {
  count++;captured_len=0;
  if(s)for(unsigned i=0;i<1024;i++) {
   if(!s[i]) {if(i)captured_len=i;break;}
   if((unsigned char)s[i]<32 || (unsigned char)s[i]>126)break;
   captured[i]=s[i];
  }
 }
 void (*notify)(const char *)=__atomic_load_n(&vendor_notify,__ATOMIC_ACQUIRE);
 if(!notify)notify=resolve_notify();
 if(notify)notify(s);
}
/* Exported to permit exact ARM64 framing + dispatch execution offline. */
int x6_gui_exchange(int fd) {
 char input[768]; W n=0; struct {int pid; W uid,gid;} peer;W size=12;
 long t=now();if(t<0)return -1;long deadline=t+2000000000L;
 if(sc(209,fd,1,17,(long)&peer,(long)&size,0) || size!=12 || peer.uid)return -1;
 if(transfer(fd,(char*)&n,4,0,deadline) || !n || n>767)return -1;
 if(transfer(fd,input,n,0,deadline))return -1;
 for(W i=0;i<n;i++)if(input[i]<32 || input[i]>126)return -1;
 input[n]=0;
 if(!dispatch || !vendor_notify)return -1;
 count=0;captured_len=0;
 __atomic_store_n(&active_tid,(int)sc(178,0,0,0,0,0,0),__ATOMIC_RELEASE);
 /* Never accept a client-supplied action name or format string. */
 int status=dispatch("DeveloperExecuteCommand_Action","{\"DeveloperExecuteCommand\": %s}",input);
 __atomic_store_n(&active_tid,0,__ATOMIC_RELEASE);
 if(status<0 || count!=1 || !captured_len)return -1;
 n=captured_len;
 if(transfer(fd,(char*)&n,4,1,deadline) || transfer(fd,captured,n,1,deadline))return -1;
 return 0;
}
static void *worker(void *arg) {
 int listener=(int)(long)arg;
 while(!__atomic_load_n(&stopping,__ATOMIC_ACQUIRE)) {
  int fd=(int)sc(242,listener,0,0,0x80800,0,0); /* accept4, nonblock accepted fd */
  if(fd==-4)continue;
  if(fd==-11) {long t=now();if(t<0)break;ready(listener,1,t+100000000L);continue;}
  if(fd<0)break;
  if(!__atomic_load_n(&stopping,__ATOMIC_ACQUIRE))x6_gui_exchange(fd);
  sc(57,fd,0,0,0,0,0);
 }
 sc(57,listener,0,0,0,0,0);return 0;
}
/* Directory provisioned by a separate tmpfiles rule; statx avoids libc ABI
 * assumptions. Do not unlink a preexisting socket or replace any file. */
int x6_owner_gui_start(void) {
 if(__atomic_exchange_n(&started,1,__ATOMIC_ACQ_REL))return -1;
 dispatch=dlsym(0,"InsDev_Native_Exec");
 __atomic_store_n(&vendor_notify,resolve_notify(),__ATOMIC_RELEASE);
 if(!dispatch || !vendor_notify || sc(175,0,0,0,0,0,0))return -1;
 /* AArch64 open flags differ from generic/x86 O_DIRECTORY/O_NOFOLLOW. */
 int dir=sc(56,-100,(long)"/run/x6-owner-gui-bridge",0x8c000,0,0,0); /* ARM64 O_DIRECTORY|NOFOLLOW|CLOEXEC */
 if(dir<0)return -1;
 unsigned char st[256]={0};
 int r=sc(291,dir,(long)"",0x1000,0x7ff,(long)st,0);sc(57,dir,0,0,0,0,0);
 W mask=*(W *)(st+0),uid=*(W *)(st+20);unsigned short mode=*(unsigned short *)(st+28);
 if(r || (mask&0xb)!=0xb || uid || mode!=040700)return -1;
 int fd=sc(198,1,0x80801,0,0,0,0);if(fd<0)return -1;
 const struct {short family;char p[sizeof(path)];} addr={1,"/run/x6-owner-gui-bridge/dispatch.sock"};
 if(sc(200,fd,(long)&addr,sizeof(addr),0,0,0) || sc(201,fd,4,0,0,0,0))goto fail;
 if(pthread_create(&worker_thread,0,worker,(void *)(long)fd))goto fail;
 thread_live=1;return 0;
fail:sc(57,fd,0,0,0,0,0);return -1;
}

/* Called before original EwDone frees the dispatcher. No detached worker may
 * retain a pointer into the vendor context across GUI teardown. */
int x6_owner_gui_stop(void) {
 __atomic_store_n(&stopping,1,__ATOMIC_RELEASE);
 if(thread_live) {
  /* Only the worker owns this fd: after worker exit its integer may already
   * belong to an unrelated GUI socket. Idle poll is bounded to 100 ms, and
   * each client exchange has an absolute 2 s I/O deadline, so stop needs no
   * cross-thread shutdown. The vendor dispatcher itself may still block. */
  if(pthread_join(worker_thread,0))return -1;
  thread_live=0;
 }
 return 0;
}
