package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/Optum/Redbox/pkg/db"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDb(t *testing.T) {
	// Load Terraform outputs
	tfOpts := &terraform.Options{
		TerraformDir: "../../modules",
	}
	tfOut := terraform.OutputAll(t, tfOpts)

	// Configure the DB service
	awsSession, err := session.NewSession()
	require.Nil(t, err)
	dbSvc := db.New(
		dynamodb.New(
			awsSession,
			aws.NewConfig().WithRegion(tfOut["aws_region"].(string)),
		),
		tfOut["dynamodb_table_account_name"].(string),
		tfOut["redbox_lease_db_table_name"].(string),
	)

	// Truncate tables, to make sure we're starting off clean
	truncateDBTables(t, dbSvc)

	t.Run("GetAccount / PutAccount", func(t *testing.T) {
		t.Run("Should retrieve an Account by Id", func(t *testing.T) {
			// Cleanup table on completion
			defer truncateAccountTable(t, dbSvc)

			// Create mock accounts
			accountIds := []string{"111", "222", "333"}
			timeNow := time.Now().Unix()
			for _, acctID := range accountIds {
				a := newAccount(acctID, timeNow)
				a.Metadata = map[string]interface{}{"hello": "world"}
				err := dbSvc.PutAccount(*a)
				require.Nil(t, err)
			}

			// Retrieve the RedboxAccount, check that it matches our mock
			acct, err := dbSvc.GetAccount("222")
			require.Nil(t, err)
			expected := newAccount("222", timeNow)
			expected.Metadata = map[string]interface{}{"hello": "world"}
			require.Equal(t, expected, acct)
		})

		t.Run("Should return Nil if no account is found", func(t *testing.T) {
			// Try getting an account that doesn't exist
			acct, err := dbSvc.GetAccount("NotAnAccount")
			require.Nil(t, err)
			require.Nil(t, acct)
		})
	})

	t.Run("GetReadyAccount", func(t *testing.T) {
		// Get the first Ready Account
		t.Run("Should retrieve an Account by Ready Status", func(t *testing.T) {
			// Cleanup table on completion
			defer truncateAccountTable(t, dbSvc)

			// Create mock accounts
			timeNow := time.Now().Unix()
			accountNotReady := db.RedboxAccount{
				ID:            "111",
				AccountStatus: "NotReady",
			}
			err := dbSvc.PutAccount(accountNotReady)
			require.Nil(t, err)
			err = dbSvc.PutAccount(*newAccount("222", timeNow)) // Ready
			require.Nil(t, err)

			// Retrieve the first Ready RedboxAccount, check that it matches our
			// mock
			acct, err := dbSvc.GetReadyAccount()
			require.Nil(t, err)
			require.Equal(t, newAccount("222", timeNow), acct)
		})

		// Return nil if no Ready Accounts are available
		t.Run("Should return Nil if no account is ready", func(t *testing.T) {
			// Try getting a ready account that doesn't exist
			acct, err := dbSvc.GetReadyAccount()
			require.Nil(t, acct)
			require.Nil(t, err)

			// Cleanup table on completion
			defer truncateAccountTable(t, dbSvc)

			// Create NotReady mock accounts
			accountNotReady := db.RedboxAccount{
				ID:            "111",
				AccountStatus: "NotReady",
			}
			err = dbSvc.PutAccount(accountNotReady)
			require.Nil(t, err)

			// Verify no account is still ready
			acct, err = dbSvc.GetReadyAccount()
			require.Nil(t, acct)
			require.Nil(t, err)
		})
	})

	t.Run("GetAccountsForReset", func(t *testing.T) {
		// Get the first Ready Account
		t.Run("Should retrieve Accounts by non-Ready Status", func(t *testing.T) {
			// Cleanup table on completion
			defer truncateAccountTable(t, dbSvc)

			// Create mock accounts
			timeNow := time.Now().Unix()
			err := dbSvc.PutAccount(*newAccount("222", timeNow)) // Ready
			require.Nil(t, err)
			accountNotReady := db.RedboxAccount{
				ID:             "222",
				AccountStatus:  "NotReady",
				LastModifiedOn: timeNow,
			}
			err = dbSvc.PutAccount(accountNotReady)
			require.Nil(t, err)
			accountLeased := db.RedboxAccount{
				ID:             "333",
				AccountStatus:  "Leased",
				LastModifiedOn: timeNow,
			}
			err = dbSvc.PutAccount(accountLeased)
			require.Nil(t, err)

			// Retrieve all RedboxAccount that can be Reset (non-Ready)
			accts, err := dbSvc.GetAccountsForReset()
			require.Nil(t, err)
			require.Equal(t, 2, len(accts))
			require.Equal(t, accountNotReady, *accts[0])
			require.Equal(t, accountLeased, *accts[1])
		})
	})

	t.Run("FindAccountsByStatus", func(t *testing.T) {

		t.Run("should return matching accounts", func(t *testing.T) {
			defer truncateAccountTable(t, dbSvc)

			// Create some accounts in the DB
			for _, acct := range []db.RedboxAccount{
				{ID: "1", AccountStatus: db.Ready},
				{ID: "2", AccountStatus: db.NotReady},
				{ID: "3", AccountStatus: db.Ready},
				{ID: "4", AccountStatus: db.Leased},
			} {
				err := dbSvc.PutAccount(acct)
				require.Nil(t, err)
			}

			// Find ready accounts
			res, err := dbSvc.FindAccountsByStatus(db.Ready)
			require.Nil(t, err)
			require.Equal(t, []*db.RedboxAccount{
				{ID: "1", AccountStatus: db.Ready},
				{ID: "3", AccountStatus: db.Ready},
			}, res)
		})

		t.Run("should return an empty list, if none match", func(t *testing.T) {
			defer truncateAccountTable(t, dbSvc)

			// Create some accounts in the DB
			for _, acct := range []db.RedboxAccount{
				{ID: "1", AccountStatus: db.NotReady},
				{ID: "2", AccountStatus: db.Leased},
			} {
				err := dbSvc.PutAccount(acct)
				require.Nil(t, err)
			}

			// Find ready accounts
			res, err := dbSvc.FindAccountsByStatus(db.Ready)
			require.Nil(t, err)
			require.Equal(t, []*db.RedboxAccount{}, res)
		})

	})

	t.Run("TransitionAccountStatus", func(t *testing.T) {
		require.NotNil(t, "TODO")
	})

	t.Run("TransitionLeaseStatus", func(t *testing.T) {

		t.Run("Should transition from one state to another", func(t *testing.T) {
			// Cleanup DB when we're done
			defer truncateLeaseTable(t, dbSvc)

			// Create a mock lease with Status=Active
			acctID := "111"
			principalID := "222"
			timeNow := time.Now().Unix()
			lease := db.RedboxLease{
				AccountID:             acctID,
				PrincipalID:           principalID,
				LeaseStatus:           db.Active,
				CreatedOn:             timeNow,
				LastModifiedOn:        timeNow,
				LeaseStatusModifiedOn: timeNow,
			}
			putAssgn, err := dbSvc.PutLease(lease)
			require.Equal(t, db.RedboxLease{}, *putAssgn) // should return an empty account lease since its new
			require.Nil(t, err)
			leaseBefore, err := dbSvc.GetLease(acctID, principalID)
			time.Sleep(1 * time.Second) // Ensure LastModifiedOn and LeaseStatusModifiedOn changes

			// Set a ResetLock on the Lease
			updatedLease, err := dbSvc.TransitionLeaseStatus(
				acctID, principalID,
				db.Active, db.ResetLock,
			)
			require.Nil(t, err)
			require.NotNil(t, updatedLease)

			// Check that the returned Lease
			// has Status=ResetLock
			require.Equal(t, updatedLease.LeaseStatus, db.ResetLock)

			// Check the lease in the db
			leaseAfter, err := dbSvc.GetLease(acctID, principalID)
			require.Nil(t, err)
			require.NotNil(t, leaseAfter)
			require.Equal(t, leaseAfter.LeaseStatus, db.ResetLock)
			require.True(t, leaseBefore.LastModifiedOn !=
				leaseAfter.LastModifiedOn)
			require.True(t, leaseBefore.LeaseStatusModifiedOn !=
				leaseAfter.LeaseStatusModifiedOn)
		})

		t.Run("Should fail if the Lease does not exit", func(t *testing.T) {
			// Attempt to lock an lease that doesn't exist
			updatedLease, err := dbSvc.TransitionLeaseStatus(
				"not-an-acct-id", "not-a-principal-id",
				db.Active, db.ResetLock,
			)
			require.Nil(t, updatedLease)
			require.NotNil(t, err)

			assert.Equal(t, "unable to update lease status from \"Active\" to \"ResetLock\" for not-an-acct-id/not-a-principal-id: "+
				"no lease exists with Status=\"Active\"", err.Error())
		})

		t.Run("Should fail if account is not in prevStatus", func(t *testing.T) {
			// Run test for each non-active status
			notActiveStatuses := []db.LeaseStatus{db.FinanceLock, db.Decommissioned}
			for _, status := range notActiveStatuses {

				t.Run(fmt.Sprint("...when status is ", status), func(t *testing.T) {
					// Cleanup DB when we're done
					defer truncateLeaseTable(t, dbSvc)

					// Create a mock lease
					// with our non-active status
					acctID := "111"
					principalID := "222"
					timeNow := time.Now().Unix()
					lease := db.RedboxLease{
						AccountID:      acctID,
						PrincipalID:    principalID,
						LeaseStatus:    status,
						CreatedOn:      timeNow,
						LastModifiedOn: timeNow,
					}
					putAssgn, err := dbSvc.PutLease(lease)
					require.Equal(t, db.RedboxLease{}, *putAssgn) // should return an empty account lease since its new
					require.Nil(t, err)

					// Attempt to set a ResetLock on the Lease
					updatedLease, err := dbSvc.TransitionLeaseStatus(
						acctID, principalID,
						db.Active, status,
					)
					require.Nil(t, updatedLease)
					require.NotNil(t, err)

					require.IsType(t, &db.StatusTransitionError{}, err)
					assert.Equal(t, fmt.Sprintf("unable to update lease status from \"Active\" to \"%v\" for 111/222: "+
						"no lease exists with Status=\"Active\"", status), err.Error())
				})

			}
		})

	})

	t.Run("TransitionAccountStatus", func(t *testing.T) {

		t.Run("Should transition from one state to another", func(t *testing.T) {
			// Cleanup DB when we're done
			defer truncateAccountTable(t, dbSvc)

			// Create a mock lease with Status=Active
			acctID := "111"
			timeNow := time.Now().Unix()
			account := db.RedboxAccount{
				ID:             acctID,
				AccountStatus:  db.Leased,
				LastModifiedOn: timeNow,
			}
			err := dbSvc.PutAccount(account)
			require.Nil(t, err)
			accountBefore, err := dbSvc.GetAccount(acctID)
			time.Sleep(1 * time.Second) // Ensure LastModifiedOn changes

			// Set a ResetLock on the Lease
			updatedAccount, err := dbSvc.TransitionAccountStatus(
				acctID,
				db.Leased, db.Ready,
			)
			require.Nil(t, err)
			require.NotNil(t, updatedAccount)

			// Check that the returned account
			// has Status=Ready
			require.Equal(t, updatedAccount.AccountStatus, db.Ready)

			// Check the account in the db got updated
			accountAfter, err := dbSvc.GetAccount(acctID)
			require.Nil(t, err)
			require.NotNil(t, accountAfter)
			require.Equal(t, accountAfter.AccountStatus, db.Ready)
			require.True(t, accountBefore.LastModifiedOn !=
				accountAfter.LastModifiedOn)
		})

		t.Run("Should fail if the Account does not exit", func(t *testing.T) {
			// Attempt to modify an account that doesn't exist
			updatedAccount, err := dbSvc.TransitionAccountStatus(
				"not-an-acct-id",
				db.NotReady, db.Ready,
			)
			require.Nil(t, updatedAccount)
			require.NotNil(t, err)

			assert.Equal(t, "unable to update account status from \"NotReady\" to \"Ready\" for account not-an-acct-id: "+
				"no account exists with Status=\"NotReady\"", err.Error())
		})

		t.Run("Should fail if account is not in prevStatus", func(t *testing.T) {
			// Run test for each status except "Ready"
			notActiveStatuses := []db.AccountStatus{db.NotReady, db.Leased}
			for _, status := range notActiveStatuses {

				t.Run(fmt.Sprint("...when status is ", status), func(t *testing.T) {
					// Cleanup DB when we're done
					defer truncateAccountTable(t, dbSvc)

					// Create a mock account
					// with our non-active status
					acctID := "111"
					account := db.RedboxAccount{
						ID:            acctID,
						AccountStatus: status,
					}
					err := dbSvc.PutAccount(account)
					require.Nil(t, err)

					// Attempt to change status from Ready -> NotReady
					// (should fail, because the account is not currently
					updatedAccount, err := dbSvc.TransitionAccountStatus(
						acctID,
						db.Ready, db.NotReady,
					)
					require.Nil(t, updatedAccount)
					require.NotNil(t, err)

					require.IsType(t, &db.StatusTransitionError{}, err)
					require.Equal(t, "unable to update account status from \"Ready\" to \"NotReady\" for account 111: "+
						"no account exists with Status=\"Ready\"", err.Error())
				})

			}
		})

	})

	t.Run("FindLeasesByAccount", func(t *testing.T) {

		t.Run("Find Existing Account", func(t *testing.T) {
			// Cleanup DB when we're done
			defer truncateLeaseTable(t, dbSvc)

			// Create a mock lease
			// with our non-active status
			acctID := "111"
			principalID := "222"
			status := db.Active
			timeNow := time.Now().Unix()
			lease := db.RedboxLease{
				AccountID:             acctID,
				PrincipalID:           principalID,
				LeaseStatus:           status,
				CreatedOn:             timeNow,
				LastModifiedOn:        timeNow,
				LeaseStatusModifiedOn: timeNow,
			}
			putAssgn, err := dbSvc.PutLease(lease)
			require.Equal(t, db.RedboxLease{}, *putAssgn) // should return an empty account lease since its new
			require.Nil(t, err)

			foundaccount, err := dbSvc.FindLeasesByAccount("111")

			require.NotNil(t, foundaccount)
			require.Nil(t, err)
		})

		t.Run("Fail to find non-existent Account", func(t *testing.T) {
			// Cleanup DB when we're done
			defer truncateLeaseTable(t, dbSvc)

			// Create a mock lease
			// with our non-active status
			acctID := "333"
			principalID := "222"
			status := db.Active
			timeNow := time.Now().Unix()
			lease := db.RedboxLease{
				AccountID:             acctID,
				PrincipalID:           principalID,
				LeaseStatus:           status,
				CreatedOn:             timeNow,
				LastModifiedOn:        timeNow,
				LeaseStatusModifiedOn: timeNow,
			}
			putAssgn, err := dbSvc.PutLease(lease)
			require.Equal(t, db.RedboxLease{}, *putAssgn) // should return an empty account lease since its new
			require.Nil(t, err)

			foundLease, err := dbSvc.FindLeasesByAccount("111")

			// require.Nil(t, foundLease)
			require.Empty(t, foundLease)
			require.Nil(t, err)
		})
	})

	t.Run("FindLeasesByPrincipal", func(t *testing.T) {

		t.Run("Find Existing Principal", func(t *testing.T) {
			// Cleanup DB when we're done
			defer truncateLeaseTable(t, dbSvc)

			// Create a mock lease
			// with our non-active status
			acctID := "111"
			principalID := "222"
			status := db.Active
			lease := db.RedboxLease{
				AccountID:   acctID,
				PrincipalID: principalID,
				LeaseStatus: status,
			}
			putAssgn, err := dbSvc.PutLease(lease)
			require.Equal(t, db.RedboxLease{}, *putAssgn) // should return an empty account lease since its new

			foundaccount, err := dbSvc.FindLeasesByPrincipal("222")

			require.NotNil(t, foundaccount)
			require.Nil(t, err)
		})

		t.Run("Fail to find non-existent Lease", func(t *testing.T) {
			// Cleanup DB when we're done
			defer truncateLeaseTable(t, dbSvc)

			// Create a mock lease
			// with our non-active status
			acctID := "333"
			principalID := "222"
			status := db.Active
			lease := db.RedboxLease{
				AccountID:   acctID,
				PrincipalID: principalID,
				LeaseStatus: status,
			}
			putAssgn, err := dbSvc.PutLease(lease)
			require.Equal(t, db.RedboxLease{}, *putAssgn) // should return an empty account lease since its new
			require.Nil(t, err)

			foundLease, err := dbSvc.FindLeasesByPrincipal("111")

			require.Nil(t, foundLease)
			require.Nil(t, err)
		})
	})

	t.Run("FindLeasesByStatus", func(t *testing.T) {

		t.Run("should return leases matching a status", func(t *testing.T) {
			defer truncateLeaseTable(t, dbSvc)

			// Create some leases in the DB
			for _, lease := range []db.RedboxLease{
				{AccountID: "1", PrincipalID: "pid", LeaseStatus: db.Active},
				{AccountID: "2", PrincipalID: "pid", LeaseStatus: db.ResetLock},
				{AccountID: "3", PrincipalID: "pid", LeaseStatus: db.Active},
				{AccountID: "4", PrincipalID: "pid", LeaseStatus: db.ResetLock},
			} {
				_, err := dbSvc.PutLease(lease)
				require.Nil(t, err)
			}

			// Find ResetLock leases
			res, err := dbSvc.FindLeasesByStatus(db.ResetLock)
			require.Nil(t, err)
			require.Equal(t, []*db.RedboxLease{
				{AccountID: "2", PrincipalID: "pid", LeaseStatus: db.ResetLock},
				{AccountID: "4", PrincipalID: "pid", LeaseStatus: db.ResetLock},
			}, res)
		})

		t.Run("should return an empty list if none match", func(t *testing.T) {
			defer truncateLeaseTable(t, dbSvc)

			// Create some leases in the DB
			for _, lease := range []db.RedboxLease{
				{AccountID: "1", PrincipalID: "pid", LeaseStatus: db.Active},
				{AccountID: "2", PrincipalID: "pid", LeaseStatus: db.Active},
			} {
				_, err := dbSvc.PutLease(lease)
				require.Nil(t, err)
			}

			// Find ResetLock leases
			res, err := dbSvc.FindLeasesByStatus(db.ResetLock)
			require.Nil(t, err)
			require.Equal(t, []*db.RedboxLease{}, res)
		})

	})

	t.Run("GetAccounts", func(t *testing.T) {
		t.Run("returns a list of accounts", func(t *testing.T) {
			defer truncateAccountTable(t, dbSvc)
			expectedID := "1234123412"
			account := *newAccount(expectedID, 1561382309)
			err := dbSvc.PutAccount(account)
			require.Nil(t, err)

			accounts, err := dbSvc.GetAccounts()
			require.Nil(t, err)
			require.True(t, true, len(accounts) > 0)
			require.Equal(t, accounts[0].ID, expectedID, "The ID of the returns record should match the expected ID")
		})
	})

	t.Run("DeleteAccount", func(t *testing.T) {
		accountID := "1234123412"

		t.Run("when the account exists", func(t *testing.T) {
			t.Run("when the account is not leased", func(t *testing.T) {
				defer truncateAccountTable(t, dbSvc)
				account := *newAccount(accountID, 1561382309)
				err := dbSvc.PutAccount(account)
				require.Nil(t, err, "it returns no errors")
				returnedAccount, err := dbSvc.DeleteAccount(accountID)
				require.Equal(t, account.ID, returnedAccount.ID, "returned account matches the deleted account")
				require.Nil(t, err, "it returns no errors on delete")
				deletedAccount, err := dbSvc.GetAccount(accountID)
				require.Nil(t, deletedAccount, "the account is deleted")
				require.Nil(t, err, "it returns no errors")
			})

			t.Run("when the account is leased", func(t *testing.T) {
				defer truncateAccountTable(t, dbSvc)
				account := db.RedboxAccount{
					ID:             accountID,
					AccountStatus:  db.Leased,
					LastModifiedOn: 1561382309,
				}
				err := dbSvc.PutAccount(account)
				require.Nil(t, err, "it should not error on delete")
				returnedAccount, err := dbSvc.DeleteAccount(accountID)
				require.Equal(t, account.ID, returnedAccount.ID, "returned account matches the deleted account")
				expectedErrorMessage := fmt.Sprintf("Unable to delete account \"%s\": account is leased.", accountID)
				require.NotNil(t, err, "it returns an error")
				assert.IsType(t, &db.AccountLeasedError{}, err)
				require.EqualError(t, err, expectedErrorMessage, "it has the correct error message")
			})
		})

		t.Run("when the account does not exists", func(t *testing.T) {
			nonexistentAccount, err := dbSvc.DeleteAccount(accountID)
			require.Nil(t, nonexistentAccount, "no account is returned")
			require.NotNil(t, err, "it returns an error")
			expectedErrorMessage := fmt.Sprintf("No account found with ID \"%s\".", accountID)
			require.EqualError(t, err, expectedErrorMessage, "it has the correct error message")
			assert.IsType(t, &db.AccountNotFoundError{}, err)
		})
	})

	t.Run("UpdateMetadata", func(t *testing.T) {
		defer truncateAccountTable(t, dbSvc)
		id := "test-metadata"
		account := db.RedboxAccount{ID: id, AccountStatus: db.Ready}
		err := dbSvc.PutAccount(account)
		require.Nil(t, err)

		expected := map[string]interface{}{
			"sso": map[string]interface{}{
				"hello": "world",
			},
		}

		err = dbSvc.UpdateMetadata(id, expected)
		require.Nil(t, err)

		updatedAccount, err := dbSvc.GetAccount(id)
		require.Equal(t, expected, updatedAccount.Metadata, "Metadata should be updated")
		require.NotEqual(t, 0, updatedAccount.LastModifiedOn, "Last modified is updated")
	})
}

func newAccount(id string, timeNow int64) *db.RedboxAccount {
	account := db.RedboxAccount{
		ID:             id,
		AccountStatus:  "Ready",
		LastModifiedOn: timeNow,
	}
	return &account
}

// Remove all records from the RedboxAccount table
func truncateAccountTable(t *testing.T, dbSvc *db.DB) {
	/*
		DynamoDB does not provide a "truncate" method.
		Instead, we need to find all records in the DB table,
		and remove them in a "BatchWrite" requests.
	*/

	// Find all records in the RedboxAccount table
	scanResult, err := dbSvc.Client.Scan(
		&dynamodb.ScanInput{
			TableName: aws.String(dbSvc.AccountTableName),
		},
	)
	require.Nil(t, err)

	if len(scanResult.Items) < 1 {
		return
	}

	// Populate a list of `DeleteRequests` for each item we found in the table
	var deleteRequests []*dynamodb.WriteRequest
	for _, item := range scanResult.Items {
		deleteRequests = append(deleteRequests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{
				Key: map[string]*dynamodb.AttributeValue{
					"Id": item["Id"],
				},
			},
		})
	}

	// Execute Batch requests, to remove all items
	_, err = dbSvc.Client.BatchWriteItem(
		&dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]*dynamodb.WriteRequest{
				dbSvc.AccountTableName: deleteRequests,
			},
		},
	)
	require.Nil(t, err)
}

/*
Remove all records from the RedboxLease table
*/
func truncateLeaseTable(t *testing.T, dbSvc *db.DB) {
	/*
		DynamoDb does not provide a "truncate" method.
		Instead, we need to find all records in the DB table,
		and remove them in a "BatchWrite" requests.
	*/

	// Find all records in the RedboxAccount table
	scanResult, err := dbSvc.Client.Scan(
		&dynamodb.ScanInput{
			TableName: aws.String(dbSvc.LeaseTableName),
		},
	)
	require.Nil(t, err)

	if len(scanResult.Items) < 1 {
		return
	}

	// Populate a list of `DeleteRequests` for each
	// item we found in the table
	var deleteRequests []*dynamodb.WriteRequest
	for _, item := range scanResult.Items {
		deleteRequests = append(deleteRequests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{
				Key: map[string]*dynamodb.AttributeValue{
					"AccountId":   item["AccountId"],
					"PrincipalId": item["PrincipalId"],
				},
			},
		})
	}

	// Execute Batch requests, to remove all items
	_, err = dbSvc.Client.BatchWriteItem(
		&dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]*dynamodb.WriteRequest{
				dbSvc.LeaseTableName: deleteRequests,
			},
		},
	)
	require.Nil(t, err)
}

func truncateDBTables(t *testing.T, dbSvc *db.DB) {
	truncateAccountTable(t, dbSvc)
	truncateLeaseTable(t, dbSvc)
}