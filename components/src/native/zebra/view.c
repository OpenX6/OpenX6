// Embedded Wizard callbacks run on the GUI thread. Only the new transparent bitmap is writable.
typedef unsigned char Obj;
typedef struct {int x,y,r,b;} Rect;
typedef struct {Obj *owner; void (*fn)(Obj*);} Slot;
typedef struct {void *pixels; int px,py;} Lock;
typedef struct {Obj *page,*button,*image,*resource;void *bitmap;unsigned tick,heartbeat;int on,shown;unsigned long stamp;unsigned char bits[36000];} State;
#define SYM __attribute__((visibility("hidden")))
extern SYM State s;
extern SYM char live_class[],capture_class[],focus_id[],focus_class[],app_id[],app_class[],button_class[],font_resource[],font_class[],image_class[],resource_class[];
extern void *get_auto(void*,void*),*new_object(void*,void*),*load_resource(void*,void*),*new_string(const char*);
extern void group_add(Obj*,Obj*,int),group_remove(Obj*,Obj*),back(Obj*,Obj*),lock_obj(Obj*),unlock_obj(Obj*),set_bounds(Obj*,Rect),set_title(Obj*,void*),set_bg(Obj*,unsigned),set_fg(Obj*,unsigned),set_drawbg(Obj*,int),set_font(Obj*,void*),set_align(Obj*,int),set_radius(Obj*,int),image_bitmap(Obj*,Obj*),image_visible(Obj*,int),set_bitmap(Obj*,void*,void*),invalidate(Obj*,Rect);
extern Slot new_slot(Obj*,void(*)(Obj*));
extern void *create_bitmap(int,unsigned long,int,int);extern Lock *lock_bitmap(void*,int,Rect,int,int);extern void unlock_bitmap(Lock*);
extern unsigned ticks(void);extern long previous(void);
#define R "/run/openx6-clip/"
static long sys(long n,long a,long b,long c,long d){register long x0 __asm__("x0")=a,x1 __asm__("x1")=b,x2 __asm__("x2")=c,x3 __asm__("x3")=d,x8 __asm__("x8")=n;__asm__ volatile("svc 0":"+r"(x0):"r"(x1),"r"(x2),"r"(x3),"r"(x8):"memory","cc");return x0;}
static void drop(const char*p){sys(35,-100,(long)p,0,0);}
static void demand(unsigned long now){long f=sys(56,-100,(long)R "demand",0xa0041,0600);if(f>=0){sys(64,f,(long)&now,8,0);sys(57,f,0,0,0);}}
static void hide(void){if(s.shown){image_visible(s.image,0);s.shown=0;}}
static void click(Obj*o){if(o!=s.button)return;s.on=!s.on;set_fg(o,s.on?0xff38dfff:0xffffffff);set_title(o,new_string(s.on?"Zebra *":"Zebra"));if(!s.on){drop(R "demand");hide();}s.heartbeat=0;}
static Obj *page(void){Obj *f=get_auto(focus_id,focus_class),*p=f?*(Obj**)(f+0x28):0;
 if(!p){Obj *a=get_auto(app_id,app_class);p=a?*(Obj**)(a+0x7e8):0;}
 return p&&*(Obj**)(p+0x30)&&!*(Obj**)(p+0xa8)&&(*(void**)p==live_class||*(void**)p==capture_class)?p:0;}
static void detach(void){if(!s.page)return;drop(R "demand");hide();if(s.button)group_remove(s.page,s.button);if(s.image)group_remove(s.page,s.image);
 if(s.resource)unlock_obj(s.resource);unlock_obj(s.page);s.page=s.button=s.image=s.resource=0;s.bitmap=0;s.heartbeat=0;}
static void attach(Obj*p){
 Obj *b=new_object(button_class,0);if(!b)return;lock_obj(p);s.page=p;group_add(p,b,0);s.button=b;
 set_title(b,new_string(s.on?"Zebra *":"Zebra"));set_drawbg(b,1);set_bg(b,s.on?0xff0069b0:0xdf343434);set_font(b,load_resource(font_resource,font_class));set_fg(b,s.on?0xff38dfff:0xffffffff);set_align(b,56);
 *(Slot*)(b+0x2e8)=new_slot(b,click);set_bounds(b,(Rect){350,277,453,335});
}
static int overlay(void){
 if(s.bitmap)return 1;
 Obj *r=new_object(resource_class,0);if(!r)return 0;lock_obj(r);
 void *raw=create_bitmap(0,((unsigned long)600<<32)|480,0,1);
 if(!raw){unlock_obj(r);return 0;}set_bitmap(r,raw,0);
 Obj *v=new_object(image_class,0);if(!v){unlock_obj(r);return 0;}
 group_add(s.page,v,0);back(s.page,v);image_visible(v,0);image_bitmap(v,r);set_bounds(v,(Rect){0,0,480,600});
 s.image=v;s.bitmap=raw;s.resource=r;return 1;
}
long entry(void){
 long ret=previous();unsigned t=ticks();if(t-s.tick<100u)return ret;s.tick=t;
 Obj *p=page();if(p!=s.page){detach();if(p)attach(p);}
 if(!s.page||!s.on)return ret;
 long ts[2];if(sys(113,1,(long)ts,0,0)<0)return ret;
 unsigned long now=ts[0]*1000UL+ts[1]/1000000;
 if(!s.heartbeat||t-s.heartbeat>=250){demand(now);s.heartbeat=t;}
 long f=sys(56,-100,(long)R "mask",0xa0000,0);unsigned long stamp=0;int good=0;
 if(f>=0){good=sys(63,f,(long)&stamp,8,0)==8&&stamp<=now&&now-stamp<=350;
 if(good&&s.shown&&s.stamp==stamp){sys(57,f,0,0,0);return ret;}
 good=good&&sys(63,f,(long)s.bits,sizeof s.bits,0)==sizeof s.bits;sys(57,f,0,0,0);}
 if(!good){hide();return ret;}if(!overlay())return ret;
 Rect screen={0,0,480,600};Lock *l=lock_bitmap(s.bitmap,0,screen,0,1);if(!l){hide();return ret;}
 if(!l->pixels||l->px!=4||l->py<1920||l->py>8192){unlock_bitmap(l);hide();return ret;}
 for(int y=0;y<600;y++){unsigned *row=(unsigned*)((char*)l->pixels+y*l->py);
 for(int x=0;x<480;x++)row[x]=(s.bits[60*y+x/8]&(1u<<(x&7)))?(((x+y)&7)<4?0xffff00ff:0xc0000000):0;}
 unlock_bitmap(l);if(!s.shown){image_visible(s.image,1);s.shown=1;}invalidate(s.page,screen);s.stamp=stamp;return ret;
}
