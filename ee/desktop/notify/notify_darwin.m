//go:build darwin
// +build darwin

// Draws from Fyne's implementation: https://github.com/fyne-io/fyne/blob/master/app/app_darwin.m

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

BOOL doSendNotification(UNUserNotificationCenter *center, NSString *title, NSString *body) {
    UNMutableNotificationContent *content = [UNMutableNotificationContent new];
    [content autorelease];
    content.title = title;
    content.body = body;

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
    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC);
    intptr_t err = dispatch_semaphore_wait(semaphore, timeout);
    if (err != 0) {
        // Timed out, remove the pending request
        [center removePendingNotificationRequestsWithIdentifiers:@[identifier]];
    }

    return success;
}

BOOL sendNotification(char *cTitle, char *cBody) {
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    NSString *title = [NSString stringWithUTF8String:cTitle];
    NSString *body = [NSString stringWithUTF8String:cBody];

    __block BOOL success = NO;
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
                success = doSendNotification(center, title, body);
            }
            dispatch_semaphore_signal(semaphore);
        }];
    });

    // Wait for completion handler to complete so that we get a correct value for `success`
    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 60 * NSEC_PER_SEC);
    dispatch_semaphore_wait(semaphore, timeout);

    return success;
}
