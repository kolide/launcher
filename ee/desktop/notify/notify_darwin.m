//go:build darwin
// +build darwin

// Draws from Fyne's implementation: https://github.com/fyne-io/fyne/blob/master/app/app_darwin.m
// TODO: log format is not JSON

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

extern void sendFallbackNotification(char *titleCStr, char *bodyCStr);

void doSendNotification(UNUserNotificationCenter *center, NSString *title, NSString *body) {
    UNMutableNotificationContent *content = [UNMutableNotificationContent new];
    [content autorelease];
    content.title = title;
    content.body = body;

    NSString *uuid = [[NSUUID UUID] UUIDString];
    NSString *identifier = [NSString stringWithFormat:@"kolide-notify-%@", uuid];
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
        content:content trigger:nil];

    [center addNotificationRequest:request withCompletionHandler:^(NSError * _Nullable error) {
        if (error != nil) {
            NSLog(@"Could not send notification: %@", error);
        }
    }];
}

void sendNotificationWithFallback(char *cTitle, char *cBody) {
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    NSString *title = [NSString stringWithUTF8String:cTitle];
    NSString *body = [NSString stringWithUTF8String:cBody];

    UNAuthorizationOptions options = UNAuthorizationOptionAlert;
    [center requestAuthorizationWithOptions:options
        completionHandler:^(BOOL granted, NSError *_Nullable error) {
            if (!granted) {
                if (error != NULL) {
                    NSLog(@"Error asking for permission to send notifications %@", error);
                    sendFallbackNotification((char *)[title UTF8String], (char *)[body UTF8String]);
                } else {
                    NSLog(@"Unable to get permission to send notifications");
                }
            } else {
                doSendNotification(center, title, body);
            }
        }];
}
