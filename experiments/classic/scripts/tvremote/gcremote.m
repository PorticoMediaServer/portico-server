// Sends Apple TV remote presses (as a game controller) to a booted simulator. Development tool only.
// usage: gcremote <udid> <key>...   keys: up down left right select menu playpause wait
#import <Foundation/Foundation.h>
#import <objc/runtime.h>
#import <objc/message.h>
#include <dlfcn.h>

static NSString *pressed = nil;

@interface Node : NSObject
@property(nonatomic, copy) NSString *path;
@end
@implementation Node
- (NSMethodSignature *)methodSignatureForSelector:(SEL)sel {
  NSString *n = NSStringFromSelector(sel);
  if ([n isEqualToString:@"value"] || [n isEqualToString:@"xAxis"] || [n isEqualToString:@"yAxis"]) return [NSMethodSignature signatureWithObjCTypes:"f@:"];
  if ([n hasPrefix:@"is"]) return [NSMethodSignature signatureWithObjCTypes:"c@:"];
  if ([n containsString:@":"]) return [NSMethodSignature signatureWithObjCTypes:"@@:@"];
  return [NSMethodSignature signatureWithObjCTypes:"@@:"];
}
- (void)forwardInvocation:(NSInvocation *)inv {
  NSString *n = NSStringFromSelector(inv.selector);
  const char *rt = inv.methodSignature.methodReturnType;
  if (rt[0] == 'f') {
    NSString *p = [NSString stringWithFormat:@"%@.%@", self.path, n];
    float v = 0;
    if (pressed && [p rangeOfString:pressed].location != NSNotFound && [n isEqualToString:@"value"]) v = 1;
    if (getenv("GCR_TRACE")) fprintf(stderr, "%s -> %f\n", p.UTF8String, v);
    [inv setReturnValue:&v];
  } else if (rt[0] == 'c') {
    char c = 0; [inv setReturnValue:&c];
  } else {
    Node *child = [Node new];
    child.path = [NSString stringWithFormat:@"%@.%@", self.path, n];
    __autoreleasing Node *r = child;
    [inv retainArguments];
    [inv setReturnValue:&r];
  }
}
@end

typedef void *(*GCFn)(id pad);

int main(int argc, const char *argv[]) {
  @autoreleasepool {
    if (argc < 3) return 2;
    dlopen("/Library/Developer/PrivateFrameworks/CoreSimulator.framework/CoreSimulator", RTLD_NOW);
    dlopen("/Applications/Xcode-beta.app/Contents/SharedFrameworks/SimulatorKit.framework/SimulatorKit", RTLD_NOW);
    GCFn gcFn = (GCFn)dlsym(RTLD_DEFAULT, "IndigoHIDMessageForGameControllerEvent");
    NSError *err = nil;
    id ctx = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(NSClassFromString(@"SimServiceContext"), sel_registerName("sharedServiceContextForDeveloperDir:error:"), @"/Applications/Xcode-beta.app/Contents/Developer", &err);
    id set = ((id (*)(id, SEL, NSError **))objc_msgSend)(ctx, sel_registerName("defaultDeviceSetWithError:"), &err);
    NSString *udid = [NSString stringWithUTF8String:argv[1]];
    id device = nil;
    for (id d in ((id (*)(id, SEL))objc_msgSend)(set, sel_registerName("devices"))) if ([[((id (*)(id, SEL))objc_msgSend)(d, sel_registerName("UDID")) UUIDString] isEqualToString:udid]) device = d;
    if (!device) { fprintf(stderr, "no device\n"); return 1; }
    id hid = ((id (*)(id, SEL, id, NSError **))objc_msgSend)([NSClassFromString(@"_TtC12SimulatorKit24SimDeviceLegacyHIDClient") alloc], sel_registerName("initWithDevice:error:"), device, &err);
    SEL sendSel = sel_registerName("sendWithMessage:freeWhenDone:completionQueue:completion:");
    dispatch_queue_t q = dispatch_queue_create("gc", DISPATCH_QUEUE_SERIAL);
    NSDictionary *map = @{@"up": @".dpad.up", @"down": @".dpad.down", @"left": @".dpad.left", @"right": @".dpad.right", @"select": @".buttonA", @"menu": @".buttonB", @"playpause": @".buttonX"};
    for (int i = 2; i < argc; i++) {
      NSString *k = [NSString stringWithUTF8String:argv[i]];
      if ([k isEqualToString:@"wait"]) { usleep(800000); continue; }
      for (int phase = 0; phase < 2; phase++) {
        pressed = phase == 0 ? map[k] : nil;
        Node *pad = [Node new]; pad.path = @"pad";
        void *msg = gcFn(pad);
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        ((void (*)(id, SEL, void *, BOOL, dispatch_queue_t, void (^)(NSError *)))objc_msgSend)(hid, sendSel, msg, YES, q, ^(NSError *e) { if (e) fprintf(stderr, "err %s\n", e.description.UTF8String); dispatch_semaphore_signal(sem); });
        dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC));
        usleep(phase == 0 ? 80000 : 300000);
      }
    }
  }
  return 0;
}
