//go:build darwin
// +build darwin

// sendNotification draws from Fyne's implementation: https://github.com/fyne-io/fyne/blob/master/app/app_darwin.m

#import "notify_darwin.h"

@implementation NotificationDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response withCompletionHandler:(void (^)(void))completionHandler {
    NSDictionary *userInfo = response.notification.request.content.userInfo;

    NSString *actionUri = userInfo[@"action_uri"];
    if ([actionUri length] != 0) {
        [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:actionUri]];
    }

    completionHandler();
}
@end

void runNotificationListenerApp(void) {
    [NSAutoreleasePool new];
    [NSApplication sharedApplication];

    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    // Define our custom notification category with actions we will want to use on notifications later
    UNNotificationAction *learnMoreAction = [UNNotificationAction actionWithIdentifier:@"LearnMoreAction"
        title:@"Learn more" options:UNNotificationActionOptionNone];

    UNNotificationCategory *category = [UNNotificationCategory categoryWithIdentifier:@"KolideNotificationCategory"
        actions:@[learnMoreAction] intentIdentifiers:@[]
        options:UNNotificationCategoryOptionNone];
    NSSet *categories = [NSSet setWithObject:category];
    [center setNotificationCategories:categories];

    if ([UNUserNotificationCenter class]) {
        NotificationDelegate *notificationDelegate = [NotificationDelegate new];
        [notificationDelegate autorelease];
        [center setDelegate:notificationDelegate];
    }

    [NSApp run];
}

void stopNotificationListenerApp(void) {
    [NSApp terminate:nil];
}

BOOL doSendNotification(UNUserNotificationCenter *center, NSString *title, NSString *body, NSString *actionUri) {
    UNMutableNotificationContent *content = [UNMutableNotificationContent new];
    [content autorelease];
    content.title = title;
    content.body = body;
    content.categoryIdentifier = @"KolideNotificationCategory";
    content.userInfo = @{@"action_uri": actionUri};

    NSString *uuid = [[NSUUID UUID] UUIDString];
    NSString *identifier = [NSString stringWithFormat:@"kolide-notify-%@", uuid];
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
        content:content trigger:nil];

    __block BOOL success = NO;
    dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);

    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        [center addNotificationRequest:request withCompletionHandler:^(NSError * _Nullable error) {
            if (error != nil) {
                NSLog(@"Could not send notification: %@", error);
            } else {
                success = YES;
            }
            dispatch_semaphore_signal(semaphore);
        }];
    });

    // Wait for completion handler to complete so that we get a correct value for `success`
    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC);
    intptr_t err = dispatch_semaphore_wait(semaphore, timeout);
    if (err != 0) {
        // Timed out, remove the pending request
        [center removePendingNotificationRequestsWithIdentifiers:@[identifier]];
    }

    return success;
}

BOOL sendNotification(char *cTitle, char *cBody, char *cActionUri) {
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    NSString *title = [NSString stringWithUTF8String:cTitle];
    NSString *body = [NSString stringWithUTF8String:cBody];
    NSString *actionUri = [NSString stringWithUTF8String:cActionUri];

    __block BOOL canSendNotification = NO;
    UNAuthorizationOptions options = (UNAuthorizationOptionAlert | UNAuthorizationStatusProvisional);
    dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);

    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        [center requestAuthorizationWithOptions:options
        completionHandler:^(BOOL granted, NSError *_Nullable error) {
            if (!granted) {
                if (error != NULL) {
                    NSLog(@"Error asking for permission to send notifications %@", error);
                } else {
                    NSLog(@"Unable to get permission to send notifications");
                }
            } else {
                canSendNotification = YES;
            }
            dispatch_semaphore_signal(semaphore);
        }];
    });

    // Wait for completion handler to complete so that we get a correct value for `canSendNotification`
    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC);
    dispatch_semaphore_wait(semaphore, timeout);

    if (canSendNotification) {
        return doSendNotification(center, title, body, actionUri);
    }

    return NO;
}
