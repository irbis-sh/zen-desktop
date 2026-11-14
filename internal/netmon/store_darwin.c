//go:build darwin

#include <dispatch/dispatch.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <SystemConfiguration/SystemConfiguration.h>

#include "_cgo_export.h"

#define STORE_NAME CFSTR("Zen network monitor")
#define COUNT(array) ((CFIndex)(sizeof(array) / sizeof((array)[0])))

struct watch {
	SCDynamicStoreRef store;
	dispatch_queue_t queue;
};

// create_array returns an array of the n keys and releases them. It returns NULL if a key
// is NULL or the array can't be made.
static CFArrayRef create_array(CFStringRef *keys, CFIndex n) {
	CFArrayRef array = NULL;
	Boolean complete = true;
	for (CFIndex i = 0; i < n; i++) {
		if (!keys[i]) complete = false;
	}
	if (complete) {
		array = CFArrayCreate(NULL, (const void **)keys, n, &kCFTypeArrayCallBacks);
	}
	for (CFIndex i = 0; i < n; i++) {
		if (keys[i]) CFRelease(keys[i]);
	}
	return array;
}

static CFStringRef global_key(CFStringRef entity) {
	return SCDynamicStoreKeyCreateNetworkGlobalEntity(NULL, kSCDynamicStoreDomainState, entity);
}

// service_pattern matches the key of entity for every network service.
static CFStringRef service_pattern(CFStringRef entity) {
	return SCDynamicStoreKeyCreateNetworkServiceEntity(NULL, kSCDynamicStoreDomainState, kSCCompAnyRegex, entity);
}

static void changed(SCDynamicStoreRef store, CFArrayRef keys, void *info) {
	netmonChanged((uintptr_t)info);
}

void *netmon_watch(uintptr_t handle) {
	struct watch *w = calloc(1, sizeof *w);
	if (!w) return NULL;

	SCDynamicStoreContext ctx = {0, (void *)handle, NULL, NULL, NULL};
	w->store = SCDynamicStoreCreate(NULL, STORE_NAME, changed, &ctx);
	if (!w->store) goto fail;

	CFStringRef keyList[] = {
		global_key(kSCEntNetIPv4),
		global_key(kSCEntNetIPv6),
		global_key(kSCEntNetDNS),
	};
	CFStringRef patternList[] = {
		service_pattern(kSCEntNetIPv4),
		service_pattern(kSCEntNetIPv6),
		service_pattern(kSCEntNetDNS),
	};
	CFArrayRef keys = create_array(keyList, COUNT(keyList));
	CFArrayRef patterns = create_array(patternList, COUNT(patternList));
	Boolean set = keys && patterns && SCDynamicStoreSetNotificationKeys(w->store, keys, patterns);
	if (keys) CFRelease(keys);
	if (patterns) CFRelease(patterns);
	if (!set) goto fail;

	w->queue = dispatch_queue_create("sh.irbis.zen.netmon", DISPATCH_QUEUE_SERIAL);
	if (!w->queue) goto fail;
	if (!SCDynamicStoreSetDispatchQueue(w->store, w->queue)) goto fail;
	return w;

fail:
	if (w->queue) dispatch_release(w->queue);
	if (w->store) CFRelease(w->store);
	free(w);
	return NULL;
}

static void nothing(void *ctx) {}

void netmon_unwatch(void *watch) {
	struct watch *w = watch;
	// The three steps go in this order, or a callback can run after this returns. Unset,
	// the store is marked as not registered, and a callback queued after that does nothing
	// (rlsPerform in configd's SCDNotifierInformViaCallback.c). The empty function then waits
	// for a callback that is already running.
	SCDynamicStoreSetDispatchQueue(w->store, NULL);
	dispatch_sync_f(w->queue, NULL, nothing);
	dispatch_release(w->queue);
	CFRelease(w->store);
	free(w);
}

struct netmon_plist netmon_read(void) {
	struct netmon_plist out = {NULL, 0, NULL};
	CFArrayRef keys = NULL, patterns = NULL;
	CFDictionaryRef values = NULL;
	CFDataRef data = NULL;

	// A session for each read can't be left broken by a restart of configd.
	SCDynamicStoreRef store = SCDynamicStoreCreate(NULL, STORE_NAME, NULL, NULL);
	if (!store) {
		out.failed = "create store session";
		return out;
	}

	CFStringRef keyList[] = {global_key(kSCEntNetDNS)};
	CFStringRef patternList[] = {service_pattern(kSCEntNetDNS)};
	keys = create_array(keyList, COUNT(keyList));
	patterns = create_array(patternList, COUNT(patternList));
	if (!keys || !patterns) {
		out.failed = "create keys";
		goto done;
	}
	values = SCDynamicStoreCopyMultiple(store, keys, patterns);
	if (!values) {
		out.failed = "copy keys";
		goto done;
	}
	// A property list leaves the walk over the dictionaries to Go.
	data = CFPropertyListCreateData(NULL, values, kCFPropertyListBinaryFormat_v1_0, 0, NULL);
	if (!data) {
		out.failed = "encode keys";
		goto done;
	}
	out.len = (size_t)CFDataGetLength(data);
	out.bytes = malloc(out.len);
	if (!out.bytes) {
		out.failed = "allocate buffer";
		goto done;
	}
	memcpy(out.bytes, CFDataGetBytePtr(data), out.len);

done:
	if (data) CFRelease(data);
	if (values) CFRelease(values);
	if (patterns) CFRelease(patterns);
	if (keys) CFRelease(keys);
	CFRelease(store);
	return out;
}
