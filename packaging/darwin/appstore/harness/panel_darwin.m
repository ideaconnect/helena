// Objective-C (ARC) backing for the harness: native NSOpenPanel folder picker
// and app-scope security-scoped bookmark create / resolve / start / stop.
// Compiled by cgo (see panel_darwin.go). Mirrors exactly what Helena's real
// internal/sandbox package will do on the `darwin && appstore` build.

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

// The URL currently under security-scoped access, retained so its matching
// -stopAccessingSecurityScopedResource can be sent. One at a time is enough
// for the harness.
static NSURL *gHeld = nil;

// A standalone (non-Fyne) binary still needs an NSApplication for AppKit panels.
// -runModal spins its own modal run loop, so no [NSApp run] is required.
static void ensureApp(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	[NSApp activateIgnoringOtherApps:YES];
}

const char *harness_pick_and_bookmark(char **pathOut, char **errOut) {
	@autoreleasepool {
		ensureApp();
		NSOpenPanel *panel = [NSOpenPanel openPanel];
		panel.canChooseDirectories = YES;
		panel.canChooseFiles = NO;
		panel.allowsMultipleSelection = NO;
		panel.canCreateDirectories = YES;
		panel.prompt = @"Use This Folder";
		panel.message = @"Pick a NEW empty folder OUTSIDE ~/Library/Containers "
		                @"(e.g. ~/Documents/helena-sbtest).";

		NSModalResponse resp = [panel runModal];
		if (resp != NSModalResponseOK) {
			return NULL; // cancelled (errOut left NULL)
		}
		NSURL *url = panel.URLs.firstObject;

		// Mint the app-scope bookmark NOW, while url still carries the powerbox
		// extension. relativeToURL:nil => app-scope (persists across launches,
		// storable anywhere the app controls). It cannot be recreated later from
		// a bare path string.
		NSError *e = nil;
		NSData *bm = [url bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
		          includingResourceValuesForKeys:nil
		                           relativeToURL:nil
		                                   error:&e];
		if (!bm) {
			if (errOut) *errOut = strdup(e.localizedDescription.UTF8String);
			return NULL;
		}
		if (pathOut) *pathOut = strdup(url.path.UTF8String); // copied before pool drains
		return strdup([bm base64EncodedStringWithOptions:0].UTF8String);
	}
}

const char *harness_resolve_start(const char *cb64, int *staleOut, char **errOut) {
	@autoreleasepool {
		NSData *data = [[NSData alloc] initWithBase64EncodedString:[NSString stringWithUTF8String:cb64]
		                                                  options:0];
		if (!data) {
			if (errOut) *errOut = strdup("bad base64 bookmark");
			return NULL;
		}
		BOOL stale = NO;
		NSError *e = nil;
		NSURL *url = [NSURL URLByResolvingBookmarkData:data
		                                      options:NSURLBookmarkResolutionWithSecurityScope
		                                relativeToURL:nil
		                          bookmarkDataIsStale:&stale
		                                        error:&e];
		if (!url) {
			if (errOut) *errOut = strdup(e ? e.localizedDescription.UTF8String
			                              : "URLByResolvingBookmarkData returned nil");
			return NULL;
		}
		if (![url startAccessingSecurityScopedResource]) {
			if (errOut)
				*errOut = strdup("startAccessingSecurityScopedResource returned NO — check the "
				                 "com.apple.security.files.bookmarks.app-scope entitlement and signing");
			return NULL;
		}
		gHeld = url; // ARC keeps this strong file-static retained until reassigned
		if (staleOut) *staleOut = stale ? 1 : 0;
		return strdup(url.path.UTF8String);
	}
}

void harness_stop(void) {
	if (gHeld) {
		[gHeld stopAccessingSecurityScopedResource];
		gHeld = nil;
	}
}
