// Factory firmware confirmation on the camera's Embedded Wizard root.
typedef unsigned char Obj;
typedef struct {int x,y,r,b;} Rect;
typedef void (*Click)(Obj*);
typedef struct {Obj *owner;Click cb;} Slot;
typedef struct {Obj *app,*v[14];unsigned stamp,last;int w,h,sent;unsigned char token[17];} State;
#pragma GCC visibility push(hidden)
extern State state;
_Static_assert(sizeof(State)<=160,"confirmation state too large");
extern char font_resource[],root_id[],root_class[],rect_class[],text_class[],button_class[],body_resource[],font_class[];
#pragma GCC visibility pop
extern long previous(void);
extern unsigned ticks(void);
extern int buffer_direction(void),direction_vertical(int);
extern Obj *get_auto(void*,void*),*new_object(void*,void*),*load_resource(void*,void*);
extern void group_add(Obj*,Obj*,int),group_remove(Obj*,Obj*),lock_obj(Obj*),unlock_obj(Obj*);
extern void set_bounds(Obj*,Rect),rect_color(Obj*,unsigned),text_color(Obj*,unsigned),text_font(Obj*,Obj*),alignment(Obj*,int);
extern Rect bounds(Obj*);
extern void *new_utf8(const char*,int);
extern void text_string(Obj*,void*),key_text(Obj*,void*),key_font(Obj*,Obj*),key_color(Obj*,unsigned),key_bg(Obj*,int),key_fg(Obj*,unsigned),key_height(Obj*,int),key_width(Obj*,int);
extern Slot new_slot(Obj*,Click);
static long sc(long nr,long a,long b,long c,long d) {
 register long x0 __asm__("x0")=a,x1 __asm__("x1")=b,x2 __asm__("x2")=c,x3 __asm__("x3")=d,x8 __asm__("x8")=nr;
 __asm__ volatile("svc 0":"+r"(x0):"r"(x1),"r"(x2),"r"(x3),"r"(x8):"memory","cc");return x0;
}
static int read_token(unsigned char *b){
 long fd=sc(56,-100,(long)"/run/openx6-stock-prompt",0xa0800,0);
 if(fd<0)return 0;
 int ok=sc(32,fd,5,0,0)==-11 && sc(63,fd,(long)b,18,0)==17 && (b[0]==1||b[0]==2);
 sc(57,fd,0,0,0);return ok;
}
static int same(unsigned char *a,unsigned char *b){for(int i=0;i<17;i++)if(a[i]!=b[i])return 0;return 1;}
static void answer(int yes){
 unsigned char b[18];
 if(state.sent||!state.app||ticks()-state.stamp<900||!read_token(b)||!same(b,state.token))return;
 b[17]=yes;
 long fd=sc(56,-100,(long)"/run/openx6-stock-answer",0xa00c1,0600);
 if(fd<0)return;
 if(sc(64,fd,(long)b,18,0)==18)state.sent=1;
 sc(57,fd,0,0,0);
}
static void left(Obj *o){if(o==state.app)answer(state.token[0]==2);}
static void right(Obj *o){if(o==state.app)answer(state.token[0]==1);}
static void *str(const char *s){int n=0;while(s[n])n++;return new_utf8(s,n);}
static void hide(void){
 if(!state.app)return;
 for(int i=13;i>=0;i--){Obj *v=state.v[i];if(v){if(*(Obj**)(v+0x30)==state.app)group_remove(state.app,v);unlock_obj(v);state.v[i]=0;}}
 unlock_obj(state.app);state.app=0;
}
static Obj *add(Obj *app, unsigned slot, char *cls, Rect area){
 Obj *view = new_object(cls,0);
 if(view){
  state.v[slot]=view;
  lock_obj(view);
  group_add(app,view,0);
  set_bounds(view,area);
 }
 return view;
}
static void label(Obj *v, Obj *font, const char *text){
 text_font(v,font);alignment(v,18);text_color(v,0xffffffff);text_string(v,str(text));
}
static int attach(Obj *app,int w,int h,unsigned char *token){
 Obj *body=load_resource(body_resource,font_class);
 Obj *small=load_resource(font_resource,font_class);
 if(!body||!small)return 0;
 state.app=app;lock_obj(app);state.w=w;state.h=h;state.sent=0;
 for(int i=0;i<17;i++)state.token[i]=token[i];
 int first=token[0]==1;
 Rect screen={0,0,w,h};
 // Touch outside a choice is consumed. A separate solid view covers the camera.
 Obj *shield=add(app,0,button_class,screen);
 if(!shield)goto fail;
 key_height(shield,h);key_width(shield,w);set_bounds(shield,screen);
 Obj *back=add(app,1,rect_class,screen);
 if(!back)goto fail;
 rect_color(back,0xff211c19);
 const char *lines[]={first?"Stock firmware (1/2)":"Confirm install (2/2)",
                      first?"原厂固件（1/2）":"安装原厂固件（2/2）",
                      "OpenX6 and root will be lost.","安装后将失去 OpenX6 和 root"};
 for(int i=0;i<4;i++){
  int y=h*(i==0?8:i==1?21:i==2?39:50)/100;
  Obj *text=add(app,2+i,text_class,(Rect){8,y,w-8,y+h*12/100});
  if(!text)goto fail;
  label(text,i<2?body:small,lines[i]);
 }
 for(int side=0;side<2;side++){
  int l=side?w/2+6:12, r=side?w-12:w/2-6;
  int top=h*70/100,bottom=h*92/100,middle=(top+bottom)/2;
  Rect hit={l,top,r,bottom};
  int slot=6+side*4,accept=first?side:!side;
  Obj *plate=add(app,slot,rect_class,hit);
  if(!plate)goto fail;
  rect_color(plate,accept?0xff554325:0xff423c37);
  Obj *caption=add(app,slot+1,text_class,(Rect){l,top,r,middle});
  if(!caption)goto fail;
  label(caption,small,accept?(first?"Next":"Install"):"Cancel");
  Obj *action=add(app,slot+2,text_class,(Rect){l,middle-3,r,bottom-3});
  if(!action)goto fail;
  label(action,body,accept?(first?"继续":"安装原厂固件"):"取消");
  Obj *key=add(app,slot+3,button_class,hit);
  if(!key)goto fail;
  // Both action labels share the same native touch target.
  key_bg(key,0);key_fg(key,0xffffffff);key_font(key,body);
  key_height(key,bottom-top);key_width(key,r-l);
  key_text(key,str(""));
  set_bounds(key,hit);
  *(Slot*)(key+0x2e8)=new_slot(app,side?right:left);
 }
 state.stamp=ticks();return 1;
fail:
 hide();return 0;
}
long confirm_entry(void){
 long r=previous();unsigned now=ticks();
 if(now-state.last<160)return r;
 state.last=now;unsigned char token[18];
 if(!read_token(token)){hide();return r;}
 Obj *g=get_auto(root_id,root_class);Obj *app=g?*(Obj**)(g+0x7d0):0;
 if(!app){hide();return r;}
 Rect box=bounds(app);int vert=direction_vertical(buffer_direction()),w=vert?480:600,h=vert?600:480;
 if(w>box.r-box.x)w=box.r-box.x;if(h>box.b-box.y)h=box.b-box.y;
 if(w<280||h<300){hide();return r;}
 if(state.app!=app||state.w!=w||state.h!=h||!same(token,state.token)){hide();attach(app,w,h,token);}
 return r;
}
