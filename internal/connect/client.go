package connect

import (
	"fmt"
	"os"
	"path/filepath"
)

// Register announces the system, activates the
// product on SCC and adds the service to the system
func Register() error {
	printInformation("register")
	err := announceOrUpdate()
	if err != nil {
		return err
	}

	installReleasePkg := true
	product := CFG.Product
	if product.isEmpty() {
		product, err = baseProduct()
		if err != nil {
			return err
		}
		installReleasePkg = false
	}

	if err = registerProduct(product, installReleasePkg); err != nil {
		return err
	}

	if product.IsBase {
		p, err := showProduct(product)
		if err != nil {
			return err
		}
		if err := registerProductTree(p); err != nil {
			return err
		}
	}
	fmt.Print(bold(greenText("\nSuccessfully registered system\n")))
	return nil
}

// registerProduct activates the product, adds the service and installs the release package
func registerProduct(product Product, installReleasePkg bool) error {
	fmt.Printf("\nActivating %s %s %s ...\n", product.Name, product.Version, product.Arch)
	service, err := activateProduct(product, CFG.Email)
	if err != nil {
		return err
	}
	fmt.Println("-> Adding service to system ...")
	if err := addService(service.URL, service.Name, !CFG.NoZypperRefresh); err != nil {
		return err
	}
	if installReleasePkg {
		fmt.Println("-> Installing release package ...")
		if err := InstallReleasePackage(product.Name); err != nil {
			return err
		}
	}
	return nil
}

// registerProductTree traverses (depth-first search) the product
// tree and registers the recommended and available products
func registerProductTree(product Product) error {
	for _, extension := range product.Extensions {
		if extension.Recommended && extension.Available {
			if err := registerProduct(extension, true); err != nil {
				return err
			}
			return registerProductTree(extension)
		}
	}
	return nil
}

// Deregister deregisters the system
func Deregister() error {
	if fileExists("/usr/sbin/registercloudguest") {
		return fmt.Errorf("SUSE::Connect::UnsupportedOperation: " +
			"De-registration is disabled for on-demand instances. " +
			"Use `registercloudguest --clean` instead.")
	}

	if !IsRegistered() {
		return ErrSystemNotRegistered
	}

	printInformation("deregister")
	if !CFG.Product.isEmpty() {
		return deregisterProduct(CFG.Product)
	}
	baseProd, _ := baseProduct()
	baseProductService, err := upgradeProduct(baseProd)
	if err != nil {
		return err
	}

	tree, err := showProduct(baseProd)
	if err != nil {
		return err
	}
	installed, _ := installedProducts()
	installedIDs := NewStringSet()
	for _, prod := range installed {
		installedIDs.Add(prod.Name)
	}

	dependencies := make([]Product, 0)
	for _, e := range tree.toExtensionsList() {
		if installedIDs.Contains(e.Name) {
			dependencies = append(dependencies, e)
		}
	}

	// reverse loop over dependencies
	for i := len(dependencies) - 1; i >= 0; i-- {
		if err := deregisterProduct(dependencies[i]); err != nil {
			return err
		}
	}

	if err := deregisterSystem(); err != nil {
		return err
	}

	if err := removeOrRefreshService(baseProductService); err != nil {
		return err
	}
	fmt.Println("\nCleaning up ...")
	if err := Cleanup(); err != nil {
		return err
	}
	fmt.Println(bold(greenText("Successfully deregistered system")))

	return nil
}

func deregisterProduct(product Product) error {
	base, err := baseProduct()
	if err != nil {
		return err
	}
	if product.ToTriplet() == base.ToTriplet() {
		return ErrBaseProductDeactivation
	}
	fmt.Printf("\nDeactivating %s %s %s ...\n", product.Name, product.Version, product.Arch)
	service, err := deactivateProduct(product)
	if err != nil {
		return err
	}
	if err := removeOrRefreshService(service); err != nil {
		return err
	}
	fmt.Println("-> Removing release package ...")
	return removeReleasePackage(product.Name)
}

// SMT provides one service for all products, removing it would remove all repositories.
// Refreshing the service instead to remove the repos of deregistered product.
func removeOrRefreshService(service Service) error {
	if service.Name == "SMT_DUMMY_NOREMOVE_SERVICE" {
		fmt.Println("-> Refreshing service ...")
		refreshAllServices()
		return nil
	}
	fmt.Println("-> Removing service from system ...")
	return removeService(service.Name)
}

// AnnounceSystem announce system via SCC/Registration Proxy
func AnnounceSystem(distroTgt string, instanceDataFile string) (string, string, error) {
	fmt.Printf(bold("\nAnnouncing system to " + CFG.BaseURL + " ...\n"))
	instanceData, err := readInstanceData(instanceDataFile)
	if err != nil {
		return "", "", err
	}
	sysInfoBody, err := makeSysInfoBody(distroTgt, CFG.Namespace, instanceData)
	if err != nil {
		return "", "", err
	}
	return announceSystem(sysInfoBody)
}

// UpdateSystem resend the system's hardware details on SCC
func UpdateSystem(distroTarget, instanceDataFile string) error {
	fmt.Printf(bold("\nUpdating system details on %s ...\n"), CFG.BaseURL)
	instanceData, err := readInstanceData(instanceDataFile)
	if err != nil {
		return err
	}
	sysInfoBody, err := makeSysInfoBody(distroTarget, CFG.Namespace, instanceData)
	if err != nil {
		return err
	}
	return updateSystem(sysInfoBody)
}

// announceOrUpdate Announces the system to the server, receiving and storing its
// credentials. When already announced, sends the current hardware details to the server
func announceOrUpdate() error {
	if IsRegistered() {
		return UpdateSystem("", "")
	}

	distroTgt := ""
	if !CFG.Product.isEmpty() {
		distroTgt = CFG.Product.distroTarget()
	}
	login, password, err := AnnounceSystem(distroTgt, CFG.InstanceDataFile)
	if err != nil {
		return err
	}
	return writeSystemCredentials(login, password)
}

// IsRegistered returns true if there is a valid credentials file
func IsRegistered() bool {
	_, err := getCredentials()
	return err == nil
}

// UpToDate Checks if API endpoint is up-to-date,
// useful when dealing with RegistrationProxy errors
func UpToDate() bool {
	return upToDate()
}

// URLDefault returns true if using https://scc.suse.com
func URLDefault() bool {
	return CFG.BaseURL == defaultBaseURL
}

func printInformation(action string) {
	var server string
	if URLDefault() {
		server = "SUSE Customer Center"
	} else {
		server = "registration proxy " + CFG.BaseURL
	}
	if action == "register" {
		fmt.Printf(bold("Registering system to %s\n"), server)
	} else {
		fmt.Printf(bold("Deregistering system from %s\n"), server)
	}
	if CFG.FsRoot != "" {
		fmt.Println("Rooted at:", CFG.FsRoot)
	}
	if CFG.Email != "" {
		fmt.Println("Using E-Mail:", CFG.Email)
	}
}

func readInstanceData(instanceDataFile string) ([]byte, error) {
	if instanceDataFile == "" {
		return nil, nil
	}
	instanceData, err := os.ReadFile(filepath.Join(CFG.FsRoot, instanceDataFile))
	if err != nil {
		return nil, err
	}
	return instanceData, nil
}

// ProductMigrations returns the online migration paths for the installed products
func ProductMigrations(installed []Product) ([]MigrationPath, error) {
	return productMigrations(installed)
}

// OfflineProductMigrations returns the offline migration paths for the installed products and target
func OfflineProductMigrations(installed []Product, targetBaseProduct Product) ([]MigrationPath, error) {
	return offlineProductMigrations(installed, targetBaseProduct)
}

// UpgradeProduct upgades the records for given product in SCC/SMT
// The service record for new product is returned
func UpgradeProduct(product Product) (Service, error) {
	return upgradeProduct(product)
}
