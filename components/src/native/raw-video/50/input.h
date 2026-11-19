static void accept_input(int req,int fd[2],unsigned long long t[2],uint64_t e0,uint64_t e1){
 uint64_t start=ns(),early=t[0]<t[1]?t[0]:t[1],deadline=early+49000000;
 pthread_mutex_lock(&mutex);
 unsigned depth=live_in-live_out,waiting=spare_in-spare_out;
 int needs_sample=req%650==45;
 int kind=needs_sample || depth>=DIRECT || (depth && start>early+12000000);
 int full=kind && waiting==spare_cap;
 Item *it=kind?&spare[spare_in%spare_cap]:&live[live_in%DIRECT];
 pthread_mutex_unlock(&mutex);
 const char *why="sparefull";
 if(!full){
  why="late";
  if(start<early+33000000){
   why="fd";
   Mapping *a=acquire(fd[0]),*b=a?acquire(fd[1]):NULL;
   if(a && b){
    Pair *p=&it->p;
    p->handles[0]=a->fd;p->handles[1]=b->fd;p->ino[0]=a->inode;p->ino[1]=b->inode;
    if(kind){
     memcpy(p->raw[0],a->map,RAW);memcpy(p->raw[1],b->map,RAW);
     p->copied=ns();release_read(a->fd);release_read(b->fd);
    }else{p->raw[0]=a->map;p->raw[1]=b->map;p->copied=ns();}
    if(p->copied<deadline){
     p->req=req;p->entered=start;p->deadline=deadline;p->exposure[0]=e0;p->exposure[1]=e1;
     it->ready_ns=early;it->stripe=0;it->elapsed_ns=0;
     pthread_mutex_lock(&mutex);
     if(kind){spare_in++;if(spare_in-spare_out>peak)peak=spare_in-spare_out;}else live_in++;
     pthread_cond_signal(&ready);pthread_mutex_unlock(&mutex);
     printf("admit,%d,%u,%u,%d,%u,%u\n",req,depth,waiting,kind,peak,spare_cap);
     return;
    }
    if(!kind){release_read(a->fd);release_read(b->fd);}
    why="copylate";
   }else if(a)release_read(a->fd);
  }
 }
 printf("in,%d,%s,0,0,%llu,%llu\n",req,why,(unsigned long long)start,(unsigned long long)ns());
}
