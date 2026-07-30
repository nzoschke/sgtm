//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdlib.h>

@interface SGTMAppDelegate : NSObject <NSApplicationDelegate>
@end

@implementation SGTMAppDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
	return YES;
}
@end

static void sgtmRunWebView(const char *url) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		SGTMAppDelegate *delegate = [[SGTMAppDelegate alloc] init];
		[NSApp setDelegate:delegate];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];

		NSRect frame = NSMakeRect(0, 0, 1280, 820);
		NSWindowStyleMask style = NSWindowStyleMaskTitled |
			NSWindowStyleMaskClosable |
			NSWindowStyleMaskMiniaturizable |
			NSWindowStyleMaskResizable;
		NSWindow *window = [[NSWindow alloc] initWithContentRect:frame
			styleMask:style
			backing:NSBackingStoreBuffered
			defer:NO];
		[window setTitle:@"SGTM"];
		[window center];

		WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
		WKWebView *webView = [[WKWebView alloc] initWithFrame:[[window contentView] bounds]
			configuration:config];
		[webView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
		[window setContentView:webView];

		NSString *urlString = [NSString stringWithUTF8String:url];
		NSURL *nsURL = [NSURL URLWithString:urlString];
		NSURLRequest *request = [NSURLRequest requestWithURL:nsURL];
		[webView loadRequest:request];

		[window makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
		[NSApp run];
	}
}
*/
import "C"

import "unsafe"

func runDashboardWebView(url string) error {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	C.sgtmRunWebView(cURL)
	return nil
}
